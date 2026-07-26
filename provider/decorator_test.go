package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingAdapter captures the Auth each call was handed, which is the
// only thing the decorator is responsible for.
type recordingAdapter struct {
	seen  []core.Auth
	calls int
}

func (r *recordingAdapter) ID() string               { return "recording" }
func (r *recordingAdapter) Name() string             { return "recording:test" }
func (r *recordingAdapter) Models() []core.ModelSpec { return nil }
func (r *recordingAdapter) AuthSchemes() []string    { return []string{"api_key"} }
func (r *recordingAdapter) Metadata() provider.Metadata {
	return provider.Metadata{Label: "recording"}
}

func (r *recordingAdapter) Generate(_ context.Context, req core.ModelRequest) (core.ModelResult, error) {
	r.calls++
	r.seen = append(r.seen, req.Auth)
	return core.ModelResult{}, nil
}

func (r *recordingAdapter) Stream(_ context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	r.calls++
	r.seen = append(r.seen, req.Auth)
	ch := make(chan core.ModelChunk)
	close(ch)
	return ch, nil
}

func (r *recordingAdapter) CountTokens(context.Context, []core.Message) (int, error) { return 0, nil }

func TestDecoratorResolvesOnEveryCall(t *testing.T) {
	// The whole point of a Decorator over a construction-time credential:
	// an OAuth token that rotates mid-run must reach the next request.
	rec := &recordingAdapter{}
	n := 0
	dec := provider.WithDecorator(rec, func(context.Context) (core.Auth, error) {
		n++
		return core.Auth{Bearer: []string{"tok-1", "tok-2", "tok-3"}[n-1]}, nil
	})

	for range 3 {
		_, err := dec.Generate(context.Background(), core.ModelRequest{})
		require.NoError(t, err)
	}
	require.Len(t, rec.seen, 3)
	assert.Equal(t, "tok-1", rec.seen[0].Bearer)
	assert.Equal(t, "tok-2", rec.seen[1].Bearer)
	assert.Equal(t, "tok-3", rec.seen[2].Bearer,
		"a rotated token must reach the next call without rebuilding the adapter")
}

func TestDecoratorAlsoCoversStream(t *testing.T) {
	// A long stream is exactly where a construction-time token expires.
	rec := &recordingAdapter{}
	dec := provider.WithDecorator(rec, func(context.Context) (core.Auth, error) {
		return core.Auth{Bearer: "streamed"}, nil
	})
	_, err := dec.Stream(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	require.Len(t, rec.seen, 1)
	assert.Equal(t, "streamed", rec.seen[0].Bearer)
}

func TestExplicitPerCallAuthOutranksTheDecorator(t *testing.T) {
	// A caller naming a credential for one request is speaking about
	// that request; the ambient decorator only fills the gaps.
	rec := &recordingAdapter{}
	dec := provider.WithDecorator(rec, func(context.Context) (core.Auth, error) {
		return core.Auth{Bearer: "ambient", Headers: map[string]string{"x-from": "decorator"}}, nil
	})

	_, err := dec.Generate(context.Background(), core.ModelRequest{
		Auth: core.Auth{Bearer: "explicit"},
	})
	require.NoError(t, err)
	require.Len(t, rec.seen, 1)
	assert.Equal(t, "explicit", rec.seen[0].Bearer)
	assert.Equal(t, "decorator", rec.seen[0].Headers["x-from"],
		"the decorator still contributes what the caller left unset")
}

func TestDecoratorErrorAbortsBeforeTheAdapterIsCalled(t *testing.T) {
	// A credential that cannot be resolved must not reach the vendor as
	// an unauthenticated request.
	rec := &recordingAdapter{}
	want := errors.New("store unavailable")
	dec := provider.WithDecorator(rec, func(context.Context) (core.Auth, error) {
		return core.Auth{}, want
	})

	_, err := dec.Generate(context.Background(), core.ModelRequest{})
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Zero(t, rec.calls, "the adapter must not be called with no credential")
}

func TestNilDecoratorReturnsTheAdapterUnchanged(t *testing.T) {
	// So a caller can pass Options.Decorator through unconditionally.
	rec := &recordingAdapter{}
	assert.Same(t, provider.Adapter(rec), provider.WithDecorator(rec, nil))
}

func TestAuthMergeDoesNotMutateEitherSide(t *testing.T) {
	// The adapter's stored Auth is shared across concurrent requests.
	base := core.Auth{APIKey: "base", Headers: map[string]string{"a": "1"}}
	override := core.Auth{Bearer: "over", Headers: map[string]string{"b": "2"}}

	got := base.Merge(override)

	assert.Equal(t, "base", got.APIKey)
	assert.Equal(t, "over", got.Bearer)
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, got.Headers)
	assert.Equal(t, map[string]string{"a": "1"}, base.Headers, "base must be untouched")
	assert.Equal(t, map[string]string{"b": "2"}, override.Headers, "override must be untouched")
}

func TestAuthTokenPrefersOAuth(t *testing.T) {
	assert.Equal(t, "bear", core.Auth{APIKey: "key", Bearer: "bear"}.Token())
	assert.Equal(t, "key", core.Auth{APIKey: "key"}.Token())
	assert.Empty(t, core.Auth{}.Token())
	assert.True(t, core.Auth{}.IsZero())
	assert.False(t, core.Auth{Headers: map[string]string{"a": "1"}}.IsZero())
}
