package frontmatter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParse pins the observable behaviour of the adrg/frontmatter
// wrapper: which delimiters it accepts, how YAML-decoded values are
// flattened to string, and how missing / malformed headers are
// surfaced. Callers in skill/ and subagent/ rely on these guarantees.
func TestParse(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    map[string]string
		body    string // empty means assert exact match
		wantErr bool
	}{
		{
			name:    "no delimiter returns full content as body",
			content: "just body text\nno frontmatter here\n",
			want:    map[string]string{},
			body:    "just body text\nno frontmatter here\n",
		},
		{
			name:    "missing closing delimiter falls back to whole content",
			content: "---\nname: foo\nstill inside, never closes",
			want:    map[string]string{},
			body:    "---\nname: foo\nstill inside, never closes",
		},
		{
			name:    "yaml simple key:value",
			content: "---\nname: greet\ndescription: says hi\n---\nbody line",
			want:    map[string]string{"name": "greet", "description": "says hi"},
			body:    "body line",
		},
		{
			name:    "yaml list value flattens with comma",
			content: "---\nallowed-tools: [read, write]\n---\nbody",
			want:    map[string]string{"allowed-tools": "read,write"},
			body:    "body",
		},
		{
			// `[bash grep]` is not a valid YAML flow sequence — flow
			// sequences require commas between items, so the YAML decoder
			// returns it as a plain string. The wrapper preserves that.
			// Documenting the behaviour so callers writing skill markdown
			// know to use `[bash, grep]` (or a block list) for sequences.
			name:    "invalid flow sequence (no commas) stays a single string",
			content: "---\ntools: [bash grep]\n---\nbody",
			want:    map[string]string{"tools": "bash grep"},
			body:    "body",
		},
		{
			name:    "keys are lower-cased",
			content: "---\nName: foo\nDESCRIPTION: bar\n---\nbody",
			want:    map[string]string{"name": "foo", "description": "bar"},
			body:    "body",
		},
		{
			name:    "boolean and integer stringify via fmt",
			content: "---\nenabled: true\nretries: 3\n---\nbody",
			want:    map[string]string{"enabled": "true", "retries": "3"},
			body:    "body",
		},
		{
			name:    "toml delimiter recognised",
			content: "+++\nname = \"toml-skill\"\n+++\nbody",
			want:    map[string]string{"name": "toml-skill"},
			body:    "body",
		},
		{
			name:    "json delimiter recognised",
			content: ";;;\n{\"name\": \"json-skill\"}\n;;;\nbody",
			want:    map[string]string{"name": "json-skill"},
			body:    "body",
		},
		{
			name:    "trailing newline after closing delimiter stripped from body",
			content: "---\nname: foo\n---\n\nbody",
			want:    map[string]string{"name": "foo"},
			body:    "\nbody",
		},
		{
			name:    "malformed yaml returns error",
			content: "---\nname: : invalid\n---\nbody",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, body, err := Parse(tc.content)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			if tc.body != "" {
				assert.Equal(t, tc.body, body)
			}
		})
	}
}

// TestList covers the legacy comma-split helper. New code can usually
// skip it because Parse already joins sequence values with ","; this
// stays for callers that operate on raw strings.
func TestList(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  []string
	}{
		{"empty returns nil", "", nil},
		{"whitespace only returns nil", "   ", nil},
		{"bracketed list", "[read, write, edit]", []string{"read", "write", "edit"}},
		{"bare comma list", "read, write, edit", []string{"read", "write", "edit"}},
		{"single item", "bash", []string{"bash"}},
		{"extra whitespace trimmed", "  read ,  write  ", []string{"read", "write"}},
		{"empty items skipped", "read,, ,write", []string{"read", "write"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, List(tc.value))
		})
	}
}

// TestParseListRoundTrip documents the invariant callers depend on:
// Parse joins YAML sequence values with "," so the legacy List helper
// recovers the original items without further parsing.
func TestParseListRoundTrip(t *testing.T) {
	input := "---\ntools: [bash, grep, glob]\n---\n"
	fields, _, err := Parse(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"bash", "grep", "glob"}, List(fields["tools"]))
}

// TestParsePreservesPrecedingNewline documents the body trim semantics
// for callers that compare body content exactly (e.g. skill.RenderTemplate).
func TestParsePreservesPrecedingNewline(t *testing.T) {
	content := "---\nname: foo\n---\nbody starts here"
	_, body, err := Parse(content)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(body, "body starts"),
		"body should not retain the newline after closing delimiter, got %q", body)
}