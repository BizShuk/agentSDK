package credential

import (
	"testing"

	"github.com/bizshuk/auth/model"
	"github.com/stretchr/testify/assert"
)

func TestToAuthProjectsOnlyRequestCredentialData(t *testing.T) {
	got := toAuth(&model.Credential{
		APIKey:      "api-key",
		AccessToken: "bearer",
		AccountID:   "account-1",
		BaseURL:     "https://construction-only.invalid",
	})

	assert.Equal(t, "api-key", got.APIKey)
	assert.Equal(t, "bearer", got.Bearer)
	assert.Equal(t, map[string]string{"ChatGPT-Account-ID": "account-1"}, got.Headers)
}

func TestToAuthHandlesNilCredential(t *testing.T) {
	assert.True(t, toAuth(nil).IsZero())
}
