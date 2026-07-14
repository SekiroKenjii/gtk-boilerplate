package scaffold_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sekiroKenjii/gtk-boilerplate/internal/scaffold"
)

// templateDir is the real template, two levels up. These tests generate from it
// rather than from a fixture, so a change to the template that would break
// generation fails here instead of in a user's terminal.
func templateDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", "template"))
	if err != nil {
		t.Fatalf("resolve template dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("template not found at %s: %v", dir, err)
	}
	return dir
}

// generatedAt pins the clock so the licence year and the release date are
// assertable instead of drifting with the calendar.
var generatedAt = time.Date(2027, 5, 9, 10, 0, 0, 0, time.UTC)

func project(dir string) scaffold.Project {
	return scaffold.Project{
		Name:          "Sound Recorder",
		AppID:         "org.gnome.SoundRecorder",
		Module:        "github.com/gnome/soundrecorder",
		Binary:        "soundrecorder",
		Author:        "Ada Lovelace",
		Dir:           dir,
		Now:           generatedAt,
		WithNotes:     true,
		WithDashboard: true,
	}
}

func read(t *testing.T, parts ...string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(data)
}

func TestGenerateRewritesModulePath(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	gomod := read(t, dir, "go.mod")
	if !strings.Contains(gomod, "module github.com/gnome/soundrecorder") {
		t.Errorf("go.mod does not declare the new module path:\n%s", gomod)
	}

	main := read(t, dir, "cmd", "app", "main.go")
	if strings.Contains(main, "github.com/example/gtkapp") {
		t.Error("main.go still imports the template's module path")
	}
	if !strings.Contains(main, "github.com/gnome/soundrecorder/internal/app") {
		t.Error("main.go does not import the new module path")
	}
}

// The replacer's argument order is the subtle part: "github.com/example/gtkapp"
// contains "gtkapp", so a naive pass would rewrite the module path twice and
// produce github.com/example/soundrecorder.
func TestGenerateDoesNotCorruptTheModulePath(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	gomod := read(t, dir, "go.mod")
	if strings.Contains(gomod, "github.com/example/soundrecorder") {
		t.Errorf("the module path was rewritten twice:\n%s", gomod)
	}
}

func TestGenerateRewritesAppIdentity(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	appinfo := read(t, dir, "internal", "app", "appinfo.go")
	for _, want := range []string{
		`ID = "org.gnome.SoundRecorder"`,
		`Name = "Sound Recorder"`,
		`Binary = "soundrecorder"`,
	} {
		if !strings.Contains(appinfo, want) {
			t.Errorf("appinfo.go is missing %s:\n%s", want, appinfo)
		}
	}
}

func TestGenerateRenamesFiles(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// The desktop entry, the AppStream metadata, the Flatpak manifest and the
	// icon are all named after the application ID. A launcher will not find
	// them otherwise.
	for _, want := range []string{
		"build/desktop/org.gnome.SoundRecorder.desktop",
		"build/desktop/org.gnome.SoundRecorder.metainfo.xml",
		"build/flatpak/org.gnome.SoundRecorder.yml",
		"assets/org.gnome.SoundRecorder.svg",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("expected %s: %v", want, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "build/desktop/com.example.GtkApp.desktop")); err == nil {
		t.Error("the template's desktop file was copied under its old name")
	}
}

func TestGenerateSetsTheVendorInMetainfo(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	meta := read(t, dir, "build", "desktop", "org.gnome.SoundRecorder.metainfo.xml")
	if !strings.Contains(meta, `<developer id="org.gnome">`) {
		t.Errorf("the developer ID was not derived from the app ID:\n%s", meta)
	}
	if strings.Contains(meta, "com.example") {
		t.Error("the metainfo still mentions com.example")
	}
	if !strings.Contains(meta, "Ada Lovelace") {
		t.Error("the author was not written into the metainfo")
	}
}

func TestGenerateWithoutNotesDropsTheModuleAndItsImport(t *testing.T) {
	dir := t.TempDir()

	p := project(dir)
	p.WithNotes = false
	// The dashboard reads notes through notesapi, so it cannot outlive it.
	p.WithDashboard = false

	if _, err := scaffold.Generate(p, templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "internal", "modules", "notes")); !os.IsNotExist(err) {
		t.Error("the notes module directory survived")
	}

	// The important half: main.go must not import or mount what is no longer
	// there, or the generated project does not compile.
	main := read(t, dir, "cmd", "app", "main.go")
	if strings.Contains(main, "modules/notes") {
		t.Errorf("main.go still imports the notes module:\n%s", main)
	}
	if strings.Contains(main, "notes.New()") {
		t.Errorf("main.go still mounts the notes module:\n%s", main)
	}
	if !strings.Contains(main, "settings.New()") {
		t.Error("main.go no longer mounts settings, which is always kept")
	}
}

func TestGenerateRefusesANonEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "important.txt"), []byte("do not lose me"), 0o644); err != nil {
		t.Fatalf("seed the directory: %v", err)
	}

	_, err := scaffold.Generate(project(dir), templateDir(t))
	if err == nil {
		t.Fatal("expected Generate to refuse a directory with files in it")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("error = %v, want it to mention the directory is not empty", err)
	}

	// And it must not have touched what was already there.
	if got := read(t, dir, "important.txt"); got != "do not lose me" {
		t.Errorf("the existing file was modified: %q", got)
	}
}

func TestGenerateSetsTheIconInitial(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	svg := read(t, dir, "assets", "org.gnome.SoundRecorder.svg")
	if !strings.Contains(svg, ">S</text>") {
		t.Errorf("the placeholder icon does not show the app's initial:\n%s", svg)
	}
}

// The licence is the one file where getting the author wrong is not cosmetic:
// it names who owns the copyright.
func TestGenerateLicenceNamesTheAuthorAndTheCurrentYear(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	licence := read(t, dir, "LICENSE")
	if !strings.Contains(licence, "Copyright (c) 2027 Ada Lovelace") {
		t.Errorf("licence does not credit the author and the generation year:\n%s",
			firstLines(licence, 5))
	}
	if strings.Contains(licence, "Your Name") {
		t.Error("the licence still carries the template's placeholder author")
	}
	// The boilerplate's own maintainer must never end up on a generated
	// project's copyright line.
	if strings.Contains(licence, "Thuong Vo") {
		t.Error("the licence credits the boilerplate's author instead of the project's")
	}
}

func TestGenerateDerivesURLsFromTheModulePath(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	meta := read(t, dir, "build", "desktop", "org.gnome.SoundRecorder.metainfo.xml")

	for _, want := range []string{
		`<url type="homepage">https://github.com/gnome/soundrecorder</url>`,
		`<url type="bugtracker">https://github.com/gnome/soundrecorder/issues</url>`,
		`<url type="vcs-browser">https://github.com/gnome/soundrecorder</url>`,
		`<release version="0.1.0" date="2027-05-09">`,
	} {
		if !strings.Contains(meta, want) {
			t.Errorf("metainfo is missing %s", want)
		}
	}
}

// The screenshot URL is deliberately left as example.com. Rewriting it with the
// homepage prefix would produce a link that looks real, resolves to nothing, and
// ships a broken image to a software centre.
func TestGenerateLeavesTheScreenshotPlaceholderAlone(t *testing.T) {
	dir := t.TempDir()

	if _, err := scaffold.Generate(project(dir), templateDir(t)); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	meta := read(t, dir, "build", "desktop", "org.gnome.SoundRecorder.metainfo.xml")

	if !strings.Contains(meta, "<image>https://example.com/screenshots/main.png</image>") {
		t.Error("the screenshot placeholder was rewritten; it must stay obviously fake")
	}
	if strings.Contains(meta, "https://github.com/gnome/soundrecorder/screenshots/main.png") {
		t.Error("the screenshot URL was fabricated from the homepage")
	}
}

func TestURLsForGitLab(t *testing.T) {
	// GitLab hides its tracker behind /-/, unlike everyone else.
	p := scaffold.Project{Module: "gitlab.com/gnome/soundrecorder"}

	if got, want := p.Homepage(), "https://gitlab.com/gnome/soundrecorder"; got != want {
		t.Errorf("Homepage() = %q, want %q", got, want)
	}
	if got, want := p.Issues(), "https://gitlab.com/gnome/soundrecorder/-/issues"; got != want {
		t.Errorf("Issues() = %q, want %q", got, want)
	}

	gh := scaffold.Project{Module: "github.com/gnome/soundrecorder"}
	if got, want := gh.Issues(), "https://github.com/gnome/soundrecorder/issues"; got != want {
		t.Errorf("Issues() = %q, want %q", got, want)
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func TestValidateAppID(t *testing.T) {
	valid := []string{
		"com.example.MyApp",
		"org.gnome.SoundRecorder",
		"io.github.user.app_name",
		"com.example.my-app",
	}
	for _, id := range valid {
		if err := scaffold.ValidateAppID(id); err != nil {
			t.Errorf("ValidateAppID(%q) = %v, want it accepted", id, err)
		}
	}

	invalid := []struct {
		id  string
		why string
	}{
		{"", "empty"},
		{"myapp", "only one element"},
		{"com..example", "two dots in a row"},
		{".com.example", "leading dot"},
		{"com.example.", "trailing dot"},
		{"com.1example.App", "an element starting with a digit"},
		{"com.example.My App", "a space"},
		{"com.example.My/App", "a slash"},
	}
	for _, tc := range invalid {
		if err := scaffold.ValidateAppID(tc.id); err == nil {
			t.Errorf("ValidateAppID(%q) was accepted; it has %s", tc.id, tc.why)
		}
	}
}

func TestSuggestions(t *testing.T) {
	tests := []struct {
		name   string
		appID  string
		binary string
		module string
	}{
		{"Sound Recorder", "com.example.SoundRecorder", "soundrecorder", "github.com/example/soundrecorder"},
		{"my-app", "com.example.MyApp", "myapp", "github.com/example/myapp"},
		{"Notes", "com.example.Notes", "notes", "github.com/example/notes"},
	}

	for _, tc := range tests {
		if got := scaffold.SuggestAppID(tc.name); got != tc.appID {
			t.Errorf("SuggestAppID(%q) = %q, want %q", tc.name, got, tc.appID)
		}
		if got := scaffold.SuggestBinary(tc.name); got != tc.binary {
			t.Errorf("SuggestBinary(%q) = %q, want %q", tc.name, got, tc.binary)
		}
		if got := scaffold.SuggestModule(tc.name); got != tc.module {
			t.Errorf("SuggestModule(%q) = %q, want %q", tc.name, got, tc.module)
		}
	}
}
