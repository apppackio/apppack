package stacks

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
)

func TestAccountAdministratorsForm_EnterEmails(t *testing.T) {
	form, adminsPtr := AccountAdministratorsForm("")
	tm := uitest.RunForm(t, form)
	// Pass the Note
	uitest.SelectFirst(tm)
	// Type emails in the Text field and submit with Ctrl+J (next in huh Text)
	tm.Type("admin@example.com\nother@example.com")
	uitest.SelectFirst(tm)
	uitest.WaitDone(t, tm)

	lines := splitLines(*adminsPtr)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "admin@example.com" {
		t.Errorf("expected 'admin@example.com', got %q", lines[0])
	}
	if lines[1] != "other@example.com" {
		t.Errorf("expected 'other@example.com', got %q", lines[1])
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a@b.com\nc@d.com", []string{"a@b.com", "c@d.com"}},
		{"a@b.com\n\nc@d.com\n", []string{"a@b.com", "c@d.com"}},
		{"  a@b.com  \n  c@d.com  ", []string{"a@b.com", "c@d.com"}},
		{"", nil},
		{"\n\n", nil},
	}

	for _, tt := range tests {
		result := splitLines(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitLines(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
			}
		}
	}
}
