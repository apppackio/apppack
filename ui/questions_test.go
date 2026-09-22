package ui

import (
	"io"
	"strings"
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
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

// TestYesNoOptions_RendersAllOptionsWhenDefaultIsNo is a regression test for
// https://github.com/apppackio/apppack/issues/181: when the default is "no"
// (the second option), huh's Select field must still render "yes" on
// initial paint. It previously scrolled "yes" off the top of the viewport
// because huh's `selectOption` sets `viewport.YOffset` to the selected
// index without clamping, and an option marked `.Selected(true)` at a
// non-zero index triggers that path. Asserting only the bound value here
// would pass against the bug (the value was always correct) — the
// regression is specifically about what's rendered, so we inspect the view.
func TestYesNoOptions_RendersAllOptionsWhenDefaultIsNo(t *testing.T) {
	t.Parallel()

	selected := BooleanAsYesNo(false)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Enabled").
				Options(YesNoOptions(false)...).
				Value(&selected),
		),
	)

	view := uitest.RenderView(form, 80, 24)

	idx := strings.Index(view, "Enabled")
	if idx == -1 {
		t.Fatalf("expected view to contain the select title, got:\n%s", view)
	}
	optionRows := view[idx:]

	if !strings.Contains(optionRows, "yes") {
		t.Errorf("expected 'yes' option to be rendered, got:\n%s", optionRows)
	}
	if !strings.Contains(optionRows, "no") {
		t.Errorf("expected 'no' option to be rendered, got:\n%s", optionRows)
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
