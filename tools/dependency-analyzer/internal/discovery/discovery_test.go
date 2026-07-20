package discovery

import "testing"

func TestParseDirectRequires(t *testing.T) {
	tests := []struct {
		name, src string
		want      []Requirement
	}{
		{"single", "require example.com/a v1.0.0\n", []Requirement{{Path: "example.com/a", Version: "v1.0.0", Direct: true}}},
		{"group", "require (\n example.com/a v1.0.0\n example.com/b v1.2.0 // indirect\n)\n", []Requirement{{Path: "example.com/a", Version: "v1.0.0", Direct: true}, {Path: "example.com/b", Version: "v1.2.0", Direct: false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDirectRequires(tt.src)
			if len(got) != len(tt.want) {
				t.Fatalf("got %#v", got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got %#v want %#v", got, tt.want)
				}
			}
		})
	}
}
func TestParseDirectRequiresIgnoresComments(t *testing.T) {
	if got := ParseDirectRequires("// require x v1\n"); len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}
