package cmd

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
	"github.com/charmbracelet/huh"
)

func TestScheduledTaskDeleteForm_SelectFirst(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("0 0 * * ? * echo hello", 0),
		huh.NewOption("0/10 * * * ? * echo world", 1),
		huh.NewOption("0 12 * * ? * echo noon", 2),
	}

	form, idxPtr := ScheduledTaskDeleteForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)
	uitest.WaitDone(t, tm)

	if *idxPtr != 0 {
		t.Errorf("expected index 0, got %d", *idxPtr)
	}
}

func TestScheduledTaskDeleteForm_SelectSecond(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("0 0 * * ? * echo hello", 0),
		huh.NewOption("0/10 * * * ? * echo world", 1),
		huh.NewOption("0 12 * * ? * echo noon", 2),
	}

	form, idxPtr := ScheduledTaskDeleteForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectNth(tm, 1)
	uitest.WaitDone(t, tm)

	if *idxPtr != 1 {
		t.Errorf("expected index 1, got %d", *idxPtr)
	}
}

func TestScheduledTaskDeleteForm_SelectLast(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("0 0 * * ? * echo hello", 0),
		huh.NewOption("0/10 * * * ? * echo world", 1),
		huh.NewOption("0 12 * * ? * echo noon", 2),
	}

	form, idxPtr := ScheduledTaskDeleteForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectNth(tm, 2)
	uitest.WaitDone(t, tm)

	if *idxPtr != 2 {
		t.Errorf("expected index 2, got %d", *idxPtr)
	}
}
