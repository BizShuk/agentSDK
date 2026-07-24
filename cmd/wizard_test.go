package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewWizardCommandDelegate(t *testing.T) {
	cmd := NewWizardCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "wizard", cmd.Name())
}
