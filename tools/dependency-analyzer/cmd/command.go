package cmd

import (
	"context"
	"flag"
	"fmt"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/discovery"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/graph"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/policy"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/report"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const VERSION = "0.1.0"

func Run(ctx context.Context, args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("dependency-analyzer", flag.ContinueOnError)
	fs.SetOutput(errout)
	workspace := fs.String("workspace", "", "go.work path")
	format := fs.String("format", "text", "text, json, or mermaid")
	output := fs.String("output", "", "output file")
	policyPath := fs.String("policy", "", "policy JSON")
	includeTests := fs.Bool("include-tests", false, "include test imports")
	includeStdlib := fs.Bool("show-stdlib", false, "include standard-library package nodes")
	includeTool := fs.Bool("include-tool-module", false, "include analyzer module")
	exclude := fs.String("exclude", "", "comma-separated module paths or directories")
	failOn := fs.String("fail-on", "", "fail at none, warning, or error")
	indent := fs.String("json-indent", "  ", "JSON indentation")
	version := fs.Bool("version", false, "print version")
	if e := fs.Parse(args); e != nil {
		return 2
	}
	if *version {
		fmt.Fprintln(out, VERSION)
		return 0
	}
	if *format != "text" && *format != "json" && *format != "mermaid" {
		fmt.Fprintln(errout, "invalid format")
		return 2
	}
	if *failOn != "" && *failOn != "none" && *failOn != "warning" && *failOn != "error" {
		fmt.Fprintln(errout, "invalid fail-on")
		return 2
	}
	var excludes []string
	if *exclude != "" {
		excludes = strings.Split(*exclude, ",")
	}
	s, e := discovery.Discover(ctx, discovery.ExecRunner{}, discovery.Options{Workspace: *workspace, IncludeTests: *includeTests, IncludeToolModule: *includeTool, Exclude: excludes})
	if e != nil {
		fmt.Fprintln(errout, e)
		return 3
	}
	a := graph.Build(s)
	if !*includeStdlib {
		filtered := a.Packages[:0]
		for _, p := range a.Packages {
			if !p.Standard {
				filtered = append(filtered, p)
			}
		}
		a.Packages = filtered
	}
	var d []policy.Diagnostic
	if *policyPath != "" {
		c, err := policy.Load(*policyPath)
		if err != nil {
			fmt.Fprintln(errout, err)
			return 2
		}
		d = policy.Evaluate(a, c)
	}
	report.SortDiagnostics(d)
	if *failOn != "" && *failOn != "none" {
		for _, x := range d {
			if (*failOn == "error" && x.Severity == "error") || (*failOn == "warning" && (x.Severity == "warning" || x.Severity == "error")) {
				returnCode := 1
				_ = returnCode
				break
			}
		}
	}
	var target io.Writer = out
	var file *os.File
	if *output != "" {
		file, e = os.CreateTemp(filepath.Dir(*output), ".dependency-analyzer-*")
		if e != nil {
			return 3
		}
		target = file
	}
	var renderErr error
	switch *format {
	case "json":
		renderErr = report.RenderJSON(target, a, d, *indent)
	case "mermaid":
		renderErr = report.RenderMermaid(target, a)
	default:
		renderErr = report.RenderText(target, a, d)
	}
	if file != nil {
		if e = file.Close(); renderErr == nil {
			renderErr = e
		}
		if renderErr == nil {
			renderErr = os.Rename(file.Name(), *output)
		} else {
			os.Remove(file.Name())
		}
	}
	if renderErr != nil {
		return 3
	}
	if *failOn != "" && *failOn != "none" {
		for _, x := range d {
			if *failOn == "error" && x.Severity == "error" {
				return 1
			}
			if *failOn == "warning" && (x.Severity == "warning" || x.Severity == "error") {
				return 1
			}
		}
	}
	return 0
}
func Main() int { return Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr) }
