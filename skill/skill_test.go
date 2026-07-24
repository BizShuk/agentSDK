package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderTemplate(t *testing.T) {
	out := RenderTemplate("Deploy {{app}} to {{env}} ({{app}})", map[string]string{"app": "web", "env": "staging"})
	assert.Equal(t, "Deploy web to staging (web)", out)
}
