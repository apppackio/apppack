package uitest

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// formModel adapts a huh form to the tea.Model interface teatest drives.
// huh's own models still use the v1 shape (Update returns huh.Model, View
// returns a string), and huh keeps its bubbletea v2 adapter internal, so we
// supply our own.
type formModel struct {
	huh.Model
}

func (m formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.Model.Update(msg)

	return formModel{Model: next}, cmd
}

func (m formModel) View() tea.View {
	return tea.NewView(m.Model.View())
}

// RunForm creates a teatest model from a huh form with an 80x24 terminal
// and waits for the form to initialize.
func RunForm(t *testing.T, form *huh.Form) *teatest.TestModel {
	t.Helper()

	tm := teatest.NewTestModel(t, formModel{Model: form}, teatest.WithInitialTermSize(80, 24))
	time.Sleep(300 * time.Millisecond)

	return tm
}

// SelectNth sends n down-arrow keys then Enter to select the nth option (0-indexed).
func SelectNth(tm *teatest.TestModel, n int) {
	for range n {
		tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// SelectFirst sends Enter to accept the default/first option.
func SelectFirst(tm *teatest.TestModel) {
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// TypeAndSubmit types text into an input field and presses Enter.
func TypeAndSubmit(tm *teatest.TestModel, text string) {
	tm.Type(text)
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
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

// RenderView drives a huh form through Init and an initial WindowSizeMsg,
// then returns its rendered View. This exercises the same code path huh
// uses on first render (no TTY or keypresses involved), which is what
// surfaces bugs in huh's initial viewport offset calculation for Select
// fields (see https://github.com/apppackio/apppack/issues/181) — those bugs
// are invisible to tests that only assert the bound value, since a keypress
// or Enter repairs the viewport before the value is read.
func RenderView(form *huh.Form, width, height int) string {
	form.Init()
	form.Update(tea.WindowSizeMsg{Width: width, Height: height})

	return form.View()
}
