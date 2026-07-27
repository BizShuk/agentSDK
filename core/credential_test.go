package core_test

import (
	"reflect"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
)

func TestAuthDoesNotCarryEndpointConfiguration(t *testing.T) {
	_, found := reflect.TypeOf(core.Auth{}).FieldByName("BaseURL")
	assert.False(t, found, "request-time credentials must not own construction-time endpoint configuration")
}
