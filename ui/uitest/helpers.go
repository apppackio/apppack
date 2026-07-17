package uitest

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/exp/teatest"
)

// RunForm creates a teatest model from a huh form with an 80x24 terminal
// and waits for the form to initialize.
func RunForm(t *testing.T, form *huh.Form) *teatest.TestModel {
	t.Helper()

	tm := teatest.NewTestModel(t, form, teatest.WithInitialTermSize(80, 24))
	time.Sleep(300 * time.Millisecond)

	return tm
}

// SelectNth sends n down-arrow keys then Enter to select the nth option (0-indexed).
func SelectNth(tm *teatest.TestModel, n int) {
	for range n {
		tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	}

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
}

// SelectFirst sends Enter to accept the default/first option.
func SelectFirst(tm *teatest.TestModel) {
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
}

// TypeAndSubmit types text into an input field and presses Enter.
func TypeAndSubmit(tm *teatest.TestModel, text string) {
	tm.Type(text)
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
}

// WaitDone signals the form to quit and waits for the final model.
// huh forms don't automatically trigger tea.Quit when complete via teatest,
// so we send QuitMsg explicitly after a brief delay for the form to process.
func WaitDone(t *testing.T, tm *teatest.TestModel) tea.Model {
	t.Helper()

	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.QuitMsg{})

	return tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
}
