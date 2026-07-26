package agentconfig_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/utils/agentconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeAcceptsCredentialKind(t *testing.T) {
	// The whole reason this field exists in the schema: Decode with
	// DisallowUnknownFields would reject it before, so a YAML hand-written
	// from config.example.yaml failed at startup.
	got, err := agentconfig.DecodeBytes([]byte(`{
		"name": "x",
		"tier": "basic",
		"model": {
			"provider": "anthropic",
			"credential_kind": "oauth"
		}
	}`))
	require.NoError(t, err)
	assert.Equal(t, core.CREDENTIAL_KIND_OAUTH, got.Model.CredentialKind)
}

func TestDecodeAbsentBlockIsOff(t *testing.T) {
	// The layer-1 rule in JSON terms: a missing key is off, an empty
	// object is on with defaults.
	off, err := agentconfig.DecodeBytes([]byte(`{"name":"x","tier":"basic"}`))
	require.NoError(t, err)
	assert.Nil(t, off.Skills)

	on, err := agentconfig.DecodeBytes([]byte(`{"name":"x","tier":"basic","skills":{}}`))
	require.NoError(t, err)
	require.NotNil(t, on.Skills)
}

func TestEncodeDecodeRoundTripIsFixedPoint(t *testing.T) {
	for _, tier := range spec.Tiers() {
		t.Run(tier, func(t *testing.T) {
			first, err := spec.Config{Name: "round", Tier: tier, Persona: "you are terse"}.Prepare()
			require.NoError(t, err)

			raw, err := agentconfig.EncodeBytes(first)
			require.NoError(t, err)

			second, err := agentconfig.DecodeBytes(raw)
			require.NoError(t, err)
			assert.Equal(t, first, second, "an expanded config must survive a write/read cycle unchanged")

			again, err := agentconfig.EncodeBytes(second)
			require.NoError(t, err)
			assert.JSONEq(t, string(raw), string(again))
		})
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := agentconfig.DecodeBytes([]byte(`{"name":"x","toolz":{}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "toolz")
}

func TestDecodeValidates(t *testing.T) {
	_, err := agentconfig.DecodeBytes([]byte(`{"name":"x","reasoning":{"style":"vibes"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown reasoning.style")
}
