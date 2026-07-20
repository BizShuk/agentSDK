package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Runner interface {
	Run(context.Context, string, []string, string, ...string) ([]byte, []byte, error)
}
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, []byte, error) {
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	c.Env = env
	var o, e bytes.Buffer
	c.Stdout = &o
	c.Stderr = &e
	err := c.Run()
	if err != nil {
		return o.Bytes(), e.Bytes(), fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(e.String()))
	}
	return o.Bytes(), e.Bytes(), nil
}

type Options struct {
	Workspace         string
	IncludeTests      bool
	IncludeToolModule bool
	Exclude           []string
}
type Workspace struct {
	Path    string
	Modules []string
}
type Module struct {
	Path, Dir, Version, GoVersion string
	Main                          bool
	Requires                      []Requirement
	Replacement                   *Replacement
	Owner                         string
}
type Requirement struct {
	Path, Version, Owner string
	Direct               bool
}
type Replacement struct {
	Path, Version, Dir string
}
type Package struct {
	ImportPath, Module string
	Standard, Test     bool
	Imports            []string
}
type Snapshot struct {
	Workspace Workspace
	Modules   []Module
	Packages  []Package
}
type workJSON struct {
	Use []struct {
		DiskPath string `json:"DiskPath"`
	} `json:"Use"`
}
type listModule struct {
	Path, Dir, Version, GoVersion string
	Main                          bool
	Replace                       *struct{ Path, Version, Dir string } `json:"Replace"`
	Require                       []struct {
		Path, Version string
		Indirect      bool
	} `json:"Require"`
}
type listPackage struct {
	ImportPath, Dir      string
	Standard             bool
	Module               *struct{ Path string }
	Imports, TestImports []string
}

func Discover(ctx context.Context, r Runner, opt Options) (Snapshot, error) {
	wp := opt.Workspace
	if wp == "" {
		b, _, e := r.Run(ctx, "", os.Environ(), "go", "env", "GOWORK")
		if e != nil {
			return Snapshot{}, e
		}
		wp = strings.TrimSpace(string(b))
	}
	if wp == "" || wp == "off" {
		return Snapshot{}, fmt.Errorf("go.work not found")
	}
	wp, _ = filepath.Abs(wp)
	root := filepath.Dir(wp)
	env := append(os.Environ(), "GOWORK=off")
	b, _, e := r.Run(ctx, root, env, "go", "work", "edit", "-json", wp)
	if e != nil {
		return Snapshot{}, e
	}
	var w workJSON
	if e = json.Unmarshal(b, &w); e != nil {
		return Snapshot{}, fmt.Errorf("decode workspace: %w", e)
	}
	s := Snapshot{Workspace: Workspace{Path: wp}}
	for _, u := range w.Use {
		p := u.DiskPath
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		p, _ = filepath.Abs(p)
		skip := !opt.IncludeToolModule && strings.HasSuffix(filepath.Clean(p), filepath.Join("tools", "dependency-analyzer"))
		for _, x := range opt.Exclude {
			skip = skip || p == x || strings.HasSuffix(p, x)
		}
		if !skip {
			s.Workspace.Modules = append(s.Workspace.Modules, p)
		}
	}
	for _, dir := range s.Workspace.Modules {
		b, _, e = r.Run(ctx, dir, env, "go", "list", "-m", "-json", "all")
		if e != nil {
			return Snapshot{}, e
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		for {
			var m listModule
			if e = dec.Decode(&m); e != nil {
				if e.Error() == "EOF" {
					break
				}
				return Snapshot{}, e
			}
			if m.Path == "" {
				continue
			}
			requires := reqs(m.Require)
			if m.Main {
				for i := range requires {
					requires[i].Owner = m.Path
				}
			}
			var replacement *Replacement
			if m.Replace != nil {
				replacement = &Replacement{Path: m.Replace.Path, Version: m.Replace.Version, Dir: m.Replace.Dir}
			}
			s.Modules = append(s.Modules, Module{Path: m.Path, Dir: dir, Version: m.Version, GoVersion: m.GoVersion, Main: m.Main, Requires: requires, Replacement: replacement, Owner: m.Path})
		}
		b, _, e = r.Run(ctx, dir, env, "go", "list", "-deps", "-json", "./...")
		if e != nil {
			return Snapshot{}, e
		}
		dec = json.NewDecoder(bytes.NewReader(b))
		for {
			var p listPackage
			if e = dec.Decode(&p); e != nil {
				if e.Error() == "EOF" {
					break
				}
				return Snapshot{}, e
			}
			if p.Module == nil || (!opt.IncludeTests && len(p.TestImports) > 0) {
				continue
			}
			im := append([]string{}, p.Imports...)
			if opt.IncludeTests {
				im = append(im, p.TestImports...)
			}
			s.Packages = append(s.Packages, Package{p.ImportPath, p.Module.Path, p.Standard, len(p.TestImports) > 0, im})
		}
	}
	return s, nil
}
func reqs(in []struct {
	Path, Version string
	Indirect      bool
}) []Requirement {
	var out []Requirement
	for _, r := range in {
		out = append(out, Requirement{Path: r.Path, Version: r.Version, Direct: !r.Indirect})
	}
	return out
}
func ParseDirectRequires(src string) []Requirement {
	var out []Requirement
	group := false
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "require (") {
			group = true
			continue
		}
		if group && t == ")" {
			group = false
			continue
		}
		if strings.HasPrefix(t, "require ") {
			t = strings.TrimSpace(strings.TrimPrefix(t, "require "))
		}
		f := strings.Fields(t)
		if len(f) >= 2 && (!strings.HasPrefix(f[0], "//")) && (group || strings.HasPrefix(strings.TrimSpace(line), "require ")) {
			out = append(out, Requirement{Path: f[0], Version: f[1], Direct: !strings.Contains(t, "// indirect")})
		}
	}
	return out
}
