package scaffold

import (
	"fmt"
	"strings"
	"unicode"
)

// Validate checks the project description, returning the first problem it finds
// in language meant for the person who typed it.
//
// The rules here are not style preferences. An application ID that GTK rejects
// produces a window with no icon and no launcher entry, and the failure is
// silent. It is worth being strict at the one moment the user can still fix it
// cheaply.
func (p Project) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("the application needs a name")
	}
	if err := ValidateAppID(p.AppID); err != nil {
		return err
	}
	if err := ValidateModule(p.Module); err != nil {
		return err
	}
	if err := ValidateBinary(p.Binary); err != nil {
		return err
	}
	if strings.TrimSpace(p.Dir) == "" {
		return fmt.Errorf("choose somewhere to put the project")
	}
	return nil
}

// ValidateAppID enforces what GTK requires of an application ID.
//
// The rules come from GApplication: at least two dot-separated elements, each
// starting with a letter, containing only letters, digits, hyphens and
// underscores, and no more than 255 characters in total.
func ValidateAppID(id string) error {
	if id == "" {
		return fmt.Errorf("the application ID is required, for example com.example.MyApp")
	}
	if len(id) > 255 {
		return fmt.Errorf("the application ID must be shorter than 256 characters")
	}
	if strings.HasPrefix(id, ".") || strings.HasSuffix(id, ".") {
		return fmt.Errorf("the application ID must not start or end with a dot")
	}

	elements := strings.Split(id, ".")
	if len(elements) < 2 {
		return fmt.Errorf("the application ID needs at least two parts, for example com.example.MyApp")
	}

	for _, e := range elements {
		if e == "" {
			return fmt.Errorf("the application ID must not contain two dots in a row")
		}
		if !unicode.IsLetter(rune(e[0])) {
			return fmt.Errorf("each part of the application ID must start with a letter: %q does not", e)
		}
		for _, r := range e {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
				return fmt.Errorf("the application ID may only contain letters, digits, hyphens and underscores: %q is not allowed", r)
			}
		}
	}
	return nil
}

// ValidateModule checks the Go module path.
func ValidateModule(path string) error {
	if path == "" {
		return fmt.Errorf("the Go module path is required, for example github.com/you/myapp")
	}
	if strings.ContainsAny(path, " \t\n\"'") {
		return fmt.Errorf("the Go module path must not contain spaces or quotes")
	}
	if strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return fmt.Errorf("the Go module path must not start or end with a slash")
	}
	return nil
}

// ValidateBinary checks the executable name.
func ValidateBinary(name string) error {
	if name == "" {
		return fmt.Errorf("the binary needs a name")
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return fmt.Errorf("the binary name may only contain letters, digits, hyphens and underscores: %q is not allowed", r)
		}
	}
	return nil
}

// SuggestAppID proposes a reverse-DNS ID from a display name.
//
// "Sound Recorder" becomes "com.example.SoundRecorder". It is a starting point
// the user is expected to correct: the vendor half is a guess, and only they
// know their domain.
func SuggestAppID(name string) string {
	camel := camelCase(name)
	if camel == "" {
		return ""
	}
	return "com.example." + camel
}

// SuggestBinary proposes an executable name from a display name.
func SuggestBinary(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == ' ' || r == '-' || r == '_':
			// Collapse separators away entirely: "Sound Recorder" becomes
			// "soundrecorder", which is what a GNOME app would be called.
		}
	}
	return b.String()
}

// SuggestModule proposes a module path from a display name.
func SuggestModule(name string) string {
	bin := SuggestBinary(name)
	if bin == "" {
		return ""
	}
	return "github.com/example/" + bin
}

func camelCase(s string) string {
	var b strings.Builder
	upper := true

	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if upper {
				b.WriteRune(unicode.ToUpper(r))
				upper = false
			} else {
				b.WriteRune(r)
			}
		default:
			upper = true
		}
	}
	return b.String()
}
