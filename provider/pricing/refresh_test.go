package pricing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/provider/pricing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRequestsAllOutputModalities(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("output_modalities")
		_, _ = w.Write([]byte(manifestFixture))
	}))
	defer server.Close()

	snapshot, err := pricing.Fetch(context.Background(), server.Client(), server.URL, time.Unix(0, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, "all", gotQuery)
	assert.Contains(t, snapshot.Models, "meta/muse-spark-1.2")
}

func TestFetchRejectsNonSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := pricing.Fetch(context.Background(), server.Client(), server.URL, time.Unix(0, 0).UTC())
	require.ErrorContains(t, err, "503")
}

func TestDiffReportsAddedChangedAndRemovedModels(t *testing.T) {
	before := pricing.Snapshot{Models: map[string]pricing.Rate{
		"old/model":     {Prompt: "1"},
		"changed/model": {Prompt: "1"},
	}}
	after := pricing.Snapshot{Models: map[string]pricing.Rate{
		"new/model":     {Prompt: "1"},
		"changed/model": {Prompt: "2"},
	}}

	diff := pricing.Compare(before, after)
	assert.Equal(t, []string{"new/model"}, diff.Added)
	assert.Equal(t, []string{"changed/model"}, diff.Changed)
	assert.Equal(t, []string{"old/model"}, diff.Removed)
}

func TestWriteSnapshotReplacesFileOnlyWithValidSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	snapshot, err := pricing.DecodeOpenRouterManifest(strings.NewReader(manifestFixture), time.Unix(0, 0).UTC())
	require.NoError(t, err)

	require.NoError(t, pricing.WriteSnapshot(path, snapshot))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"meta/muse-spark-1.2"`)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWriteSnapshotRejectsEmptyModelsWithoutTouchingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))

	err := pricing.WriteSnapshot(path, pricing.Snapshot{Models: map[string]pricing.Rate{}})
	require.ErrorContains(t, err, "empty")
	raw, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(raw))
}

func TestReadSnapshotAllowsInitialEmptyBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
      "source":"https://openrouter.ai/api/v1/models?output_modalities=all",
      "pricing_as_of":"",
      "models":{}
    }`), 0o600))

	snapshot, err := pricing.ReadSnapshot(path)
	require.NoError(t, err)
	assert.Empty(t, snapshot.Models)
}
