package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/example/gtkapp/internal/modules/notes/domain"
	"github.com/example/gtkapp/internal/platform/errs"
)

// The domain imports neither GTK nor a database, so its tests need neither. This
// is the payoff of keeping the rules in their own layer: they run in
// milliseconds, headless, on any machine.

var testTime = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func TestNewNoteTrimsTitle(t *testing.T) {
	n, err := domain.NewNote("  Shopping list  ", "milk", testTime)
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}

	if n.Title != "Shopping list" {
		t.Errorf("title = %q, want %q", n.Title, "Shopping list")
	}
	if !n.CreatedAt.Equal(testTime) || !n.UpdatedAt.Equal(testTime) {
		t.Errorf("timestamps = %v/%v, want both %v", n.CreatedAt, n.UpdatedAt, testTime)
	}
}

func TestNewNoteRejectsBlankTitle(t *testing.T) {
	for _, title := range []string{"", "   ", "\t\n"} {
		_, err := domain.NewNote(title, "body", testTime)
		if err == nil {
			t.Fatalf("NewNote(%q): expected an error", title)
		}
		if got := errs.KindOf(err); got != errs.Invalid {
			t.Errorf("NewNote(%q): kind = %v, want Invalid", title, got)
		}
	}
}

func TestNewNoteRejectsOverlongTitle(t *testing.T) {
	long := strings.Repeat("a", domain.MaxTitleLength+1)

	_, err := domain.NewNote(long, "", testTime)
	if err == nil {
		t.Fatal("expected an error for an overlong title")
	}
	if got := errs.KindOf(err); got != errs.Invalid {
		t.Errorf("kind = %v, want Invalid", got)
	}

	// The boundary itself has to be allowed, or the limit is off by one.
	if _, err := domain.NewNote(strings.Repeat("a", domain.MaxTitleLength), "", testTime); err != nil {
		t.Errorf("a title of exactly MaxTitleLength was rejected: %v", err)
	}
}

func TestRenameBumpsUpdatedAt(t *testing.T) {
	n, err := domain.NewNote("Before", "", testTime)
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}

	later := testTime.Add(time.Hour)
	if err := n.Rename("After", later); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if n.Title != "After" {
		t.Errorf("title = %q, want %q", n.Title, "After")
	}
	if !n.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", n.UpdatedAt, later)
	}
	if !n.CreatedAt.Equal(testTime) {
		t.Errorf("CreatedAt changed to %v; it must not move", n.CreatedAt)
	}
}

func TestRenameRejectsBlankAndLeavesNoteIntact(t *testing.T) {
	n, err := domain.NewNote("Keep me", "", testTime)
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}

	if err := n.Rename("  ", testTime.Add(time.Hour)); err == nil {
		t.Fatal("expected an error for a blank title")
	}

	// A rejected rename must not half-apply: the note keeps its old title and
	// its old timestamp.
	if n.Title != "Keep me" {
		t.Errorf("title = %q after a failed rename, want it unchanged", n.Title)
	}
	if !n.UpdatedAt.Equal(testTime) {
		t.Errorf("UpdatedAt moved on a failed rename")
	}
}

func TestSummary(t *testing.T) {
	tests := []struct {
		name string
		body string
		max  int
		want string
	}{
		{"empty body", "", 10, "Empty note"},
		{"whitespace only", "  \n\t ", 10, "Empty note"},
		{"short body", "milk", 10, "milk"},
		{"newlines collapse", "milk\neggs\nbread", 20, "milk eggs bread"},
		{"truncates", "abcdefghijklmnop", 5, "abcde…"},
		{"exact length is not truncated", "abcde", 5, "abcde"},
		{"truncates by rune, not byte", "áéíóúüñ", 3, "áéí…"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := domain.Note{Body: tc.body}
			if got := n.Summary(tc.max); got != tc.want {
				t.Errorf("Summary(%d) = %q, want %q", tc.max, got, tc.want)
			}
		})
	}
}
