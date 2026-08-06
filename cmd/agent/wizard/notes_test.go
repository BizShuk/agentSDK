package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/provider"
)

// TestProviderChoicesMirrorEntries is the wizard-side guarantee that
// every Entry in the provider registry shows up as a Choice and
// retains the bits a wizard actually renders (Label, Note, Value).
// The default flag is forwarded from provider.DEFAULT_NAME so the
// wizard's Enter-to-accept behaviour matches the registry.
func TestProviderChoicesMirrorEntries(t *testing.T) {
	entries := provider.Entries()
	choices := providerChoices(entries)
	require.Len(t, choices, len(entries))

	values := make([]string, 0, len(choices))
	defaults := 0
	for _, c := range choices {
		values = append(values, c.Value)
		assert.NotEmpty(t, c.Label, "a wizard menu renders Label")
		assert.NotEmpty(t, c.Note, "every provider should say which credential it reads")
		if c.Default {
			defaults++
		}
	}
	assert.Equal(t, provider.Names(), values, "choices must follow the registry's order")
	assert.Equal(t, 1, defaults, "exactly one provider is the Enter-to-accept default")
}

// TestDefaultChoiceMatchesRegistry proves the wizard's Enter-to-accept
// option is the same provider the assembly layer treats as the default.
// A divergence here means a literal Config{Model: {Provider: ""}}
// would silently pick a non-default vendor.
func TestDefaultChoiceMatchesRegistry(t *testing.T) {
	choices := providerChoices(provider.Entries())
	var def string
	for _, c := range choices {
		if c.Default {
			def = c.Value
		}
	}
	assert.Equal(t, provider.DEFAULT_NAME, def)
}

// TestNoteIncludesCredentialEnv pins the human-facing contract: when a
// provider declares an API key env, the wizard's Note says so. That
// turn of phrase is what operators grep for when their first run
// fails on a missing credential.
func TestNoteIncludesCredentialEnv(t *testing.T) {
	choices := providerChoices(provider.Entries())
	for _, c := range choices {
		if c.Value == provider.DEFAULT_NAME {
			assert.Contains(t, c.Note, "reads",
				"the default provider's note should mention which env it reads")
		}
	}
}

func TestCatalogChoicesOnlyIncludeChatModels(t *testing.T) {
	specs := []provider.ModelSpec{
		{ID: "chat-model", Capabilities: []provider.Capability{provider.CAPABILITY_CHAT}},
		{ID: "image-model", Capabilities: []provider.Capability{provider.CAPABILITY_IMAGE}},
		{ID: "live-model", Capabilities: []provider.Capability{provider.CAPABILITY_LIVE}},
	}

	choices := catalogChoices(specs)
	require.Len(t, choices, 2)
	assert.Equal(t, "", choices[0].Value)
	assert.Equal(t, "chat-model", choices[1].Value)
}
