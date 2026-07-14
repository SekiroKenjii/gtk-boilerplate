// Package scaffold turns the template project into a real one.
//
// The template under template/ is a working Go module: it compiles, its tests
// run, and CI builds it on every push. That is deliberate. A scaffolding tool
// whose template is a pile of .tmpl files nobody can compile drifts out of date
// within a month, and the first thing a new user meets is a build error.
//
// So generation is not template expansion. It is a copy with a rename. The
// template's identifiers (its module path, application ID, name and binary) are
// replaced with yours, in file contents and in file names alike.
package scaffold

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The identifiers the template uses for itself. Generation rewrites each of
// these into whatever the user asked for.
//
// They are ordinary, valid values rather than placeholders like {{.AppID}},
// which is what lets the template compile as it stands.
const (
	tplModule = "github.com/example/gtkapp"
	tplAppID  = "com.example.GtkApp"
	tplName   = "GtkApp"
	tplBinary = "gtkapp"
	tplAuthor = "Your Name"
	tplVendor = "com.example" // the app ID minus its final element

	// The AppStream metadata and the licence carry placeholders of their own.
	tplHomepage = "https://example.com"
	tplIssues   = "https://example.com/issues"
	tplRepo     = "https://example.com/source"
	tplCopyYear = "(c) 2026"
	tplRelease  = "2026-01-01"

	// The screenshot is the one placeholder that survives generation. Nothing
	// in the project description says where a screenshot of an app that does
	// not exist yet would live, and inventing a plausible-looking URL would be
	// worse than leaving an obviously fake one: a broken image in a software
	// centre is a bug report, whereas example.com is a to-do the metainfo
	// comment already spells out.
	tplScreenshot = "https://example.com/screenshots/main.png"
)

// Project is what the user wants built.
type Project struct {
	// Name is the display name: "Sound Recorder". Shown in window titles, the
	// About dialog and the launcher.
	Name string

	// AppID is the reverse-DNS application ID: "org.gnome.SoundRecorder". GTK
	// keys the window's desktop-entry match, the GSettings path and the icon
	// name off this, so it has to be right.
	AppID string

	// Module is the Go module path: "github.com/you/soundrecorder".
	Module string

	// Binary is the executable name: "soundrecorder".
	Binary string

	// Author is the developer name shown in the About dialog and the AppStream
	// metadata.
	Author string

	// Dir is where the project is written. It must not already contain files.
	Dir string

	// Now stamps the licence's copyright year and the first AppStream release
	// date. Leave it zero and generation reads the wall clock; tests set it so
	// their assertions do not change on New Year's Eve.
	Now time.Time

	// Modules selects which example modules to keep. The settings module is
	// always kept: the shell's About dialog and the theme both depend on it,
	// and a project that starts with no way to change a preference is a project
	// whose first job is to add one.
	WithNotes     bool
	WithDashboard bool
}

// Vendor is the application ID minus its last element: the "org.gnome" of
// "org.gnome.SoundRecorder". AppStream wants it as the developer ID.
func (p Project) Vendor() string {
	if i := strings.LastIndex(p.AppID, "."); i > 0 {
		return p.AppID[:i]
	}
	return p.AppID
}

// Initial is the first letter of the name, for the placeholder icon.
func (p Project) Initial() string {
	for _, r := range p.Name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "A"
}

// Homepage is the project's web address, derived from the Go module path.
//
// A Go module path is already a URL with the scheme filed off: that is how `go
// get` finds the code. So github.com/you/recorder becomes
// https://github.com/you/recorder, and the AppStream metadata gets a link that
// actually resolves instead of example.com.
func (p Project) Homepage() string {
	return "https://" + strings.TrimSuffix(p.Module, "/")
}

// Issues is the bug tracker.
//
// GitHub, Codeberg and the rest put it at /issues. GitLab is the odd one out
// and hides it behind /-/, which is worth special-casing because a bug tracker
// link that 404s is one nobody files a bug about.
func (p Project) Issues() string {
	home := p.Homepage()
	if strings.HasPrefix(p.Module, "gitlab.com/") {
		return home + "/-/issues"
	}
	return home + "/issues"
}

// Year is the copyright year on the generated licence.
func (p Project) Year() string {
	return strconv.Itoa(p.clock().Year())
}

// ReleaseDate is the date on the first AppStream release entry.
func (p Project) ReleaseDate() string {
	return p.clock().Format("2006-01-02")
}

func (p Project) clock() time.Time {
	if p.Now.IsZero() {
		return time.Now()
	}
	return p.Now
}

// Result reports what generation produced.
type Result struct {
	Dir   string
	Files int
}

// Generate writes the project described by p, reading the template from
// templateDir.
//
// It refuses to write into a directory that already has files in it. Scaffolding
// on top of existing work is how people lose it, and "the directory is not
// empty" is a much better outcome than a silently half-merged tree.
func Generate(p Project, templateDir string) (Result, error) {
	if err := p.Validate(); err != nil {
		return Result{}, err
	}

	if err := ensureEmptyDir(p.Dir); err != nil {
		return Result{}, err
	}

	if _, err := os.Stat(filepath.Join(templateDir, "go.mod")); err != nil {
		return Result{}, fmt.Errorf(
			"%s does not look like the project template (no go.mod): %w", templateDir, err)
	}

	rep := p.replacer()
	written := 0

	err := filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(templateDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		if skip(rel, d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !p.keep(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// File and directory names carry the identifiers too:
		// com.example.GtkApp.desktop has to become org.gnome.SoundRecorder.desktop.
		target := filepath.Join(p.Dir, rep.Replace(rel))

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if err := copyFile(path, target, rep, p); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		written++
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	if err := p.writeMainGo(); err != nil {
		return Result{}, err
	}

	return Result{Dir: p.Dir, Files: written}, nil
}

// replacer rewrites the template's identifiers into the project's.
//
// Order matters, and it is longest-first on purpose. strings.Replacer matches in
// argument order at each position without rescanning what it just wrote, so the
// module path has to be tried before "gtkapp", and the full app ID before the
// vendor prefix it starts with. Get the order wrong and
// "github.com/example/gtkapp" turns into "github.com/example/soundrecorder".
//
// The screenshot rule is the same trick used defensively: it maps the URL to
// itself, purely so the generic https://example.com rule below cannot chew off
// its prefix and leave behind a link that looks real and is not.
func (p Project) replacer() *strings.Replacer {
	return strings.NewReplacer(
		tplModule, p.Module,
		tplAppID, p.AppID,

		tplScreenshot, tplScreenshot,
		tplIssues, p.Issues(),
		tplRepo, p.Homepage(),
		tplHomepage, p.Homepage(),

		tplCopyYear, "(c) "+p.Year(),
		tplRelease, p.ReleaseDate(),

		tplName, p.Name,
		tplBinary, p.Binary,
		tplAuthor, p.Author,
		tplVendor, p.Vendor(),
	)
}

// keep decides whether a path survives the module selection.
func (p Project) keep(rel string) bool {
	rel = filepath.ToSlash(rel)

	if !p.WithNotes && strings.HasPrefix(rel, "internal/modules/notes") {
		return false
	}
	if !p.WithDashboard && strings.HasPrefix(rel, "internal/modules/dashboard") {
		return false
	}
	return true
}

// writeMainGo rewrites the composition root to mount exactly the modules the
// user chose.
//
// The template's main.go mounts all three, so dropping a module's directory
// without touching main.go would leave an import of a package that no longer
// exists. That is a generated project which does not compile, and it is the one
// thing a scaffolding tool must never produce.
func (p Project) writeMainGo() error {
	var imports, mounts []string

	if p.WithDashboard {
		imports = append(imports, fmt.Sprintf("\t\"%s/internal/modules/dashboard\"", p.Module))
	}
	if p.WithNotes {
		imports = append(imports, fmt.Sprintf("\t\"%s/internal/modules/notes\"", p.Module))
	}
	imports = append(imports, fmt.Sprintf("\t\"%s/internal/modules/settings\"", p.Module))

	// Mount order decides migration order and reverse-shutdown order. Settings
	// first: it owns preferences the others read.
	mounts = append(mounts, "\t\tsettings.New(),")
	if p.WithNotes {
		mounts = append(mounts, "\t\tnotes.New(),")
	}
	if p.WithDashboard {
		mounts = append(mounts, "\t\tdashboard.New(),")
	}

	path := filepath.Join(p.Dir, "cmd", "app", "main.go")
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated main.go: %w", err)
	}

	out, err := rewriteModuleList(string(src), p.Module, imports, mounts)
	if err != nil {
		return fmt.Errorf("rewrite main.go: %w", err)
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

// rewriteModuleList swaps the module imports and the Mount call in main.go.
//
// It is deliberately a text edit rather than an AST rewrite: the input is a file
// this repository owns and tests, not arbitrary Go, and go/printer would reflow
// comments we went to some trouble to write.
func rewriteModuleList(src, module string, imports, mounts []string) (string, error) {
	// Every module import sits on its own line and shares a prefix, so they can
	// be located exactly rather than guessed at.
	prefix := fmt.Sprintf("\t\"%s/internal/modules/", module)

	// Drop the template's module imports and splice the chosen ones in where
	// the first of them was, which keeps them inside the import block and in
	// the group gofmt already put them in.
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines)+len(imports))
	spliced := false

	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			if !spliced {
				out = append(out, imports...)
				spliced = true
			}
			continue
		}
		out = append(out, line)
	}
	if !spliced {
		return "", fmt.Errorf("no module imports found; expected lines starting %q", prefix)
	}

	joined := strings.Join(out, "\n")

	// Now the Mount call.
	const mountOpen = "\tkernel.Mount(\n"
	start := strings.Index(joined, mountOpen)
	if start < 0 {
		return "", fmt.Errorf("no kernel.Mount( call found")
	}
	rest := joined[start+len(mountOpen):]
	end := strings.Index(rest, "\t)\n")
	if end < 0 {
		return "", fmt.Errorf("kernel.Mount( is not closed")
	}

	return joined[:start+len(mountOpen)] +
		strings.Join(mounts, "\n") + "\n" +
		rest[end:], nil
}

// skip drops paths that must never reach a generated project.
func skip(rel string, d fs.DirEntry) bool {
	base := filepath.Base(rel)

	switch base {
	case ".git", "bin", "vendor", "node_modules", ".flatpak-builder":
		return true
	case "coverage.out", ".DS_Store":
		return true
	}
	// Build output from someone running `make` inside the template.
	if strings.HasPrefix(filepath.ToSlash(rel), "build/flatpak-build") {
		return true
	}
	return false
}

// copyFile writes one file, rewriting identifiers in text and leaving binaries
// byte-for-byte.
func copyFile(src, dst string, rep *strings.Replacer, p Project) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if isText(data) {
		out := rep.Replace(string(data))

		// The placeholder icon spells out the template's initial. Nothing else
		// in the tree hard-codes a single letter, so a targeted swap is safer
		// than adding "G" to the replacer and hitting every G in the repository.
		if strings.HasSuffix(dst, ".svg") {
			out = strings.Replace(out, ">G</text>", ">"+p.Initial()+"</text>", 1)
		}

		return os.WriteFile(dst, []byte(out), info.Mode().Perm())
	}

	return writeRaw(src, dst, info.Mode().Perm())
}

// isText reports whether data is UTF-8 text safe to run through the replacer.
//
// A NUL byte is the giveaway: no valid UTF-8 text file contains one, and every
// binary format we might ship (an icon, a compiled resource) does.
func isText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for _, b := range data[:min(len(data), 8000)] {
		if b == 0 {
			return false
		}
	}
	return utf8.Valid(data)
}

func writeRaw(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func ensureEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return fmt.Errorf("read destination %s: %w", dir, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("destination %s is not empty", dir)
	}
	return nil
}
