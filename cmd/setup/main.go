// Command setup creates a new project from the boilerplate.
//
// Run it with no arguments and it opens a GTK window that asks what you are
// building. Give it --name and --app-id and it does the same thing without a
// display, which is what CI needs.
//
//	./setup.sh
//	./setup.sh --name "Sound Recorder" --app-id org.gnome.SoundRecorder --dir ~/src/recorder
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sekiroKenjii/gtk-boilerplate/internal/scaffold"
	"github.com/sekiroKenjii/gtk-boilerplate/internal/wizardui"
)

func main() {
	var (
		name        = flag.String("name", "", "display name, e.g. \"Sound Recorder\"")
		appID       = flag.String("app-id", "", "application ID, e.g. org.gnome.SoundRecorder")
		module      = flag.String("module", "", "Go module path, e.g. github.com/you/recorder")
		binary      = flag.String("binary", "", "executable name (default: derived from the name)")
		author      = flag.String("author", "", "author name for the About dialog and AppStream metadata")
		dir         = flag.String("dir", "", "where to write the project")
		noNotes     = flag.Bool("no-notes", false, "leave out the example notes module")
		noDashboard = flag.Bool("no-dashboard", false, "leave out the example dashboard module")
		batch       = flag.Bool("batch", false, "do not open a window; requires --name and --dir")
		templateDir = flag.String("template", "", "path to the project template (default: alongside this repository)")
	)

	flag.Usage = usage
	flag.Parse()

	tpl, err := resolveTemplate(*templateDir)
	if err != nil {
		fail(err)
	}

	defaults := scaffold.Project{
		Name:          *name,
		AppID:         *appID,
		Module:        *module,
		Binary:        *binary,
		Author:        *author,
		Dir:           *dir,
		WithNotes:     !*noNotes,
		WithDashboard: !*noDashboard && !*noNotes,
	}

	if *batch {
		if err := generate(defaults, tpl); err != nil {
			fail(err)
		}
		return
	}

	os.Exit(wizardui.Run(wizardui.Options{
		TemplateDir: tpl,
		Defaults:    defaults,
	}))
}

// generate is the headless path: no window, no questions, no guessing beyond
// what can be derived from the name.
func generate(p scaffold.Project, templateDir string) error {
	if p.Name == "" {
		return fmt.Errorf("--batch needs --name")
	}
	if p.Dir == "" {
		return fmt.Errorf("--batch needs --dir")
	}

	// Fill in what the wizard would have suggested, so a caller who only has a
	// name still gets a coherent project.
	if p.AppID == "" {
		p.AppID = scaffold.SuggestAppID(p.Name)
	}
	if p.Module == "" {
		p.Module = scaffold.SuggestModule(p.Name)
	}
	if p.Binary == "" {
		p.Binary = scaffold.SuggestBinary(p.Name)
	}
	if p.Author == "" {
		p.Author = "Your Name"
	}

	abs, err := filepath.Abs(p.Dir)
	if err != nil {
		return err
	}
	p.Dir = abs

	result, err := scaffold.Generate(p, templateDir)
	if err != nil {
		return err
	}

	fmt.Printf("Created %s\n", result.Dir)
	fmt.Printf("  %-12s %s\n", "name", p.Name)
	fmt.Printf("  %-12s %s\n", "app id", p.AppID)
	fmt.Printf("  %-12s %s\n", "module", p.Module)
	fmt.Printf("  %-12s %s\n", "binary", p.Binary)
	fmt.Printf("  %-12s %d\n", "files", result.Files)
	fmt.Printf("\nNext:\n  cd %s\n  make run\n", result.Dir)
	return nil
}

// resolveTemplate finds the template directory.
//
// The wizard reads the template off disk rather than from an embedded copy,
// because Go refuses to embed a directory that is a module of its own. The
// template has to be a module of its own, or it could not compile and be tested
// as the real thing. See internal/scaffold.
func resolveTemplate(override string) (string, error) {
	if override != "" {
		return check(override)
	}

	// The usual case: the binary was built from this repository and run from it.
	if wd, err := os.Getwd(); err == nil {
		if dir, ok := findUp(wd); ok {
			return dir, nil
		}
	}

	// Run from somewhere else, e.g. an installed binary.
	if exe, err := os.Executable(); err == nil {
		if dir, ok := findUp(filepath.Dir(exe)); ok {
			return dir, nil
		}
	}

	return "", fmt.Errorf(
		"cannot find the project template: run this from the boilerplate repository, or pass --template")
}

// findUp walks up from start looking for a template/ directory that holds a Go
// module.
func findUp(start string) (string, bool) {
	dir := start
	for {
		candidate := filepath.Join(dir, "template")
		if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func check(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return "", fmt.Errorf("%s does not look like the template (no go.mod)", abs)
	}
	return abs, nil
}

func usage() {
	fmt.Fprint(os.Stderr, `setup: create a GTK4 + Go project from the boilerplate

Usage:
  setup                 open the graphical wizard
  setup --batch ...     create a project without a window

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprint(os.Stderr, `
Examples:
  setup
  setup --batch --name "Sound Recorder" --app-id org.gnome.SoundRecorder \
        --module github.com/you/recorder --dir ~/src/recorder
`)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "setup:", err)
	os.Exit(1)
}
