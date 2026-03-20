package cmd

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
	"github.com/charmbracelet/huh"
)

func TestShellTaskSelectForm_SelectFirst(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("web: abc123", 0),
		huh.NewOption("worker: def456", 1),
		huh.NewOption("web: ghi789", 2),
	}

	form, idxPtr := ShellTaskSelectForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)
	uitest.WaitDone(t, tm)

	if *idxPtr != 0 {
		t.Errorf("expected index 0, got %d", *idxPtr)
	}
}

func TestShellTaskSelectForm_SelectSecond(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("web: abc123", 0),
		huh.NewOption("worker: def456", 1),
		huh.NewOption("web: ghi789", 2),
	}

	form, idxPtr := ShellTaskSelectForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectNth(tm, 1)
	uitest.WaitDone(t, tm)

	if *idxPtr != 1 {
		t.Errorf("expected index 1, got %d", *idxPtr)
	}
}

func TestShellTaskSelectForm_SelectLast(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("web: abc123", 0),
		huh.NewOption("worker: def456", 1),
		huh.NewOption("web: ghi789", 2),
	}

	form, idxPtr := ShellTaskSelectForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectNth(tm, 2)
	uitest.WaitDone(t, tm)

	if *idxPtr != 2 {
		t.Errorf("expected index 2, got %d", *idxPtr)
	}
}
