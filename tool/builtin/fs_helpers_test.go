package builtin

import (
	"encoding/json"
	"path/filepath"
	"testing"

	sdkcore "github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

// testPolicy returns a Policy that allows paths under dirs.
func testPolicy(dirs ...string) *tool.Policy {
	p := tool.DefaultPolicy()
	for _, d := range dirs {
		p.AllowedPathPrefixes = append(p.AllowedPathPrefixes, d)
		if resolved, err := filepath.EvalSymlinks(d); err == nil && resolved != d {
			p.AllowedPathPrefixes = append(p.AllowedPathPrefixes, resolved)
		}
	}
	return p
}

func mustMarshal(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func unmarshalOutput[T any](t *testing.T, res sdkcore.ToolResult) T {
	t.Helper()
	raw, err := json.Marshal(res.Output)
	require.NoError(t, err)
	var out T
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}
