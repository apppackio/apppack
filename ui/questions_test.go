package ui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
)

func TestBooleanAsYesNo(t *testing.T) {
	t.Parallel()

	if BooleanAsYesNo(true) != "yes" {
		t.Error("expected yes for true")
	}

	if BooleanAsYesNo(false) != "no" {
		t.Error("expected no for false")
	}
}

func TestYesNoOptions(t *testing.T) {
	t.Parallel()

	opts := YesNoOptions(true)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options, got %d", len(opts))
	}

	opts = YesNoOptions(false)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options, got %d", len(opts))
	}
}

func TestYesNoToBool(t *testing.T) {
	t.Parallel()

	if !YesNoToBool("yes") {
		t.Error("expected true for yes")
	}

	if YesNoToBool("no") {
		t.Error("expected false for no")
	}

	if YesNoToBool("anything") {
		t.Error("expected false for non-yes value")
	}
}

func TestRunFormAccessible(t *testing.T) {
	t.Parallel()

	var name string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name").Value(&name),
		),
	).WithAccessible(true).
		WithInput(strings.NewReader("Alice\n")).
		WithOutput(io.Discard)

	if err := form.Run(); err != nil {
		t.Fatal(err)
	}

	if name != "Alice" {
		t.Errorf("expected Alice, got %s", name)
	}
}
