package configfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bizshuk/agentsdk/utils/configfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatOf(t *testing.T) {
	cases := []struct {
		name string
		path string
		want configfile.Format
	}{
		{"json extension", "agent.json", configfile.FORMAT_JSON},
		{"json extension uppercase", "AGENT.JSON", configfile.FORMAT_JSON},
		{"yaml extension", "agent.yaml", configfile.FORMAT_YAML},
		{"yml extension", "agent.yml", configfile.FORMAT_YAML},
		{"extensionless", "agent", configfile.FORMAT_YAML},
		{"stdout", "-", configfile.FORMAT_YAML},
		{"unknown extension falls back to yaml", "agent.toml", configfile.FORMAT_YAML},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, configfile.FormatOf(tc.path))
		})
	}
}

func TestReadJSON(t *testing.T) {
	cases := []struct {
		name string
		file string
		body string
		want string
	}{
		{"yaml converts to json", "c.yaml", "name: a\ntools:\n  builtin: [read]\n", `{"name":"a","tools":{"builtin":["read"]}}`},
		{"json passes through", "c.json", `{"name":"a"}`, `{"name":"a"}`},
		{"empty yaml becomes empty object", "c.yaml", "", `{}`},
		{"empty block is preserved", "c.yaml", "name: a\ntools: {}\n", `{"name":"a","tools":{}}`},
		// YAML is a superset of JSON, so a JSON body on the YAML branch
		// parses identically. Extension drives encoding, not content.
		{"json body on the yaml branch", "c.yaml", `{"name":"a"}`, `{"name":"a"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.file)
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o644))

			got, err := configfile.ReadJSON(path)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestReadJSONMissingFile(t *testing.T) {
	_, err := configfile.ReadJSON(filepath.Join(t.TempDir(), "absent.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configfile: read")
}

func TestWriteRefusesToClobber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	require.NoError(t, configfile.Write(path, []byte(`{"name":"a"}`), false))

	err := configfile.Write(path, []byte(`{"name":"b"}`), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// force overwrites, and the path is reported absolute so the message
	// is unambiguous when the caller ran from another directory.
	require.NoError(t, configfile.Write(path, []byte(`{"name":"b"}`), true))
	got, err := configfile.ReadJSON(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"b"}`, string(got))
}

func TestWriteEncodesByExtension(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`{"name":"a","tools":{"builtin":["read"]}}`)

	yamlPath := filepath.Join(dir, "c.yaml")
	require.NoError(t, configfile.Write(yamlPath, body, false))
	raw, err := os.ReadFile(yamlPath)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "name: a", "a .yaml path must produce YAML")
	assert.NotContains(t, string(raw), `"name"`)

	jsonPath := filepath.Join(dir, "c.json")
	require.NoError(t, configfile.Write(jsonPath, body, false))
	raw, err = os.ReadFile(jsonPath)
	require.NoError(t, err)
	assert.JSONEq(t, string(body), string(raw), "a .json path must pass JSON through verbatim")
}

func TestRoundTripIsFixedPoint(t *testing.T) {
	body := `{"name":"a","limits":{"max_turns":3},"tools":{}}`
	for _, ext := range []string{".yaml", ".json"} {
		t.Run(ext, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "c"+ext)
			require.NoError(t, configfile.Write(path, []byte(body), false))

			got, err := configfile.ReadJSON(path)
			require.NoError(t, err)
			assert.JSONEq(t, body, string(got))
		})
	}
}
