package uitest

import (
	"testing"

	"github.com/charmbracelet/huh"
)

func newTestSelect(selected *int) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Pick:").
				Options(
					huh.NewOption("A", 0),
					huh.NewOption("B", 1),
					huh.NewOption("C", 2),
				).
				Value(selected),
		),
	)
}

func TestSelectFirst(t *testing.T) {
	var selected int
	tm := RunForm(t, newTestSelect(&selected))
	SelectFirst(tm)
	WaitDone(t, tm)

	if selected != 0 {
		t.Errorf("expected 0, got %d", selected)
	}
}

func TestSelectNth(t *testing.T) {
	var selected int
	tm := RunForm(t, newTestSelect(&selected))
	SelectNth(tm, 2)
	WaitDone(t, tm)

	if selected != 2 {
		t.Errorf("expected 2, got %d", selected)
	}
}

func TestTypeAndSubmit(t *testing.T) {
	var name string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name:").Value(&name),
		),
	)

	tm := RunForm(t, form)
	TypeAndSubmit(tm, "Alice")
	WaitDone(t, tm)

	if name != "Alice" {
		t.Errorf("expected Alice, got %s", name)
	}
}
