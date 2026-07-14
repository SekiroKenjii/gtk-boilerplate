// Command archlint enforces the module boundaries this codebase is built on.
//
// A modular monolith only stays modular if something checks. Left to code review
// alone, one "just this once" import from a screen straight into another
// module's service is all it takes, and a year later the modules are a single
// tangle with directories between them.
//
// So this runs in CI, and in `make lint`. It reads the import graph with `go
// list` and fails on any edge that breaks a rule below.
//
// # The rules
//
//  1. A module may not import another module's internals. Only its api package.
//
//     internal/modules/notes/ui  ->  internal/modules/settings/api      allowed
//     internal/modules/notes/ui  ->  internal/modules/settings/service  rejected
//
//     This is the rule the whole architecture rests on. The api package is a
//     promise; everything behind it is free to change.
//
//  2. An api package may not import another module at all, not even another
//     api. Contracts that reference each other are not contracts, they are a
//     cycle waiting to be discovered.
//
//  3. The platform may not import a module. Dependencies point inward: modules
//     know about the platform, the platform knows nothing about them.
//
//  4. The kernel (internal/app) may not import a module. Only cmd/ may, and only
//     to mount them.
//
// # Adding a rule
//
// Rules are data, in the check function. If your project grows a convention
// worth keeping ("nothing outside store/ may import database", say), add it
// there rather than trusting everyone to remember.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// pkg is the slice of `go list -json` output we care about.
type pkg struct {
	ImportPath string
	Imports    []string
}

// violation is one rejected import edge.
type violation struct {
	From   string
	To     string
	Reason string
}

func main() {
	module, err := modulePath()
	if err != nil {
		fatal(err)
	}

	pkgs, err := listPackages()
	if err != nil {
		fatal(err)
	}

	violations := check(module, pkgs)
	if len(violations) == 0 {
		fmt.Printf("archlint: %d packages, no boundary violations\n", len(pkgs))
		return
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].From != violations[j].From {
			return violations[i].From < violations[j].From
		}
		return violations[i].To < violations[j].To
	})

	fmt.Fprintf(os.Stderr, "archlint: %d boundary violation(s)\n\n", len(violations))
	for _, v := range violations {
		fmt.Fprintf(os.Stderr, "  %s\n      imports %s\n      %s\n\n",
			trim(module, v.From), trim(module, v.To), v.Reason)
	}
	os.Exit(1)
}

func check(module string, pkgs []pkg) []violation {
	var out []violation

	modulesPrefix := module + "/internal/modules/"
	platformPrefix := module + "/internal/platform/"
	kernelPrefix := module + "/internal/app"

	for _, p := range pkgs {
		fromModule := moduleOf(p.ImportPath, modulesPrefix)
		fromIsAPI := fromModule != "" && isAPI(p.ImportPath, modulesPrefix, fromModule)
		fromIsPlatform := strings.HasPrefix(p.ImportPath, platformPrefix)
		fromIsKernel := p.ImportPath == kernelPrefix || strings.HasPrefix(p.ImportPath, kernelPrefix+"/")

		for _, imp := range p.Imports {
			toModule := moduleOf(imp, modulesPrefix)
			if toModule == "" {
				// Not an import of a feature module. Nothing here forbids a
				// module from using the platform or the standard library.
				continue
			}
			toIsAPI := isAPI(imp, modulesPrefix, toModule)

			switch {
			case fromIsPlatform:
				out = append(out, violation{p.ImportPath, imp,
					"the platform must not depend on a module: dependencies point inward"})

			case fromIsKernel:
				out = append(out, violation{p.ImportPath, imp,
					"the kernel must not depend on a module: only cmd/ may mount modules"})

			case fromIsAPI && toModule != fromModule:
				out = append(out, violation{p.ImportPath, imp,
					"an api package must not depend on another module: contracts that reference each other are a cycle"})

			case fromModule != "" && toModule != fromModule && !toIsAPI:
				out = append(out, violation{p.ImportPath, imp,
					fmt.Sprintf("module %q may only reach module %q through its api package", fromModule, toModule)})
			}
		}
	}

	return out
}

// moduleOf returns the feature module a package belongs to, or "" when the
// package is not part of one.
func moduleOf(importPath, modulesPrefix string) string {
	if !strings.HasPrefix(importPath, modulesPrefix) {
		return ""
	}
	rest := strings.TrimPrefix(importPath, modulesPrefix)
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[:i]
	}
	return rest
}

// isAPI reports whether importPath is a module's api package (or something
// under it).
func isAPI(importPath, modulesPrefix, module string) bool {
	prefix := modulesPrefix + module + "/api"
	return importPath == prefix || strings.HasPrefix(importPath, prefix+"/")
}

func listPackages() ([]pkg, error) {
	// -deps is deliberately absent: we only want this module's own packages,
	// not the transitive closure of the standard library and every dependency.
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	// `go list -json` emits a stream of concatenated objects, not an array.
	var pkgs []pkg
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("parse go list output: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

func modulePath() (string, error) {
	out, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		return "", fmt.Errorf("go list -m: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// trim shortens an import path for display: the module prefix is the same on
// every line and carries no information.
func trim(module, importPath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(importPath, module), "/")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "archlint:", err)
	os.Exit(1)
}
