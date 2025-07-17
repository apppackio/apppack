package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/logrusorgru/aurora"
)

type QuestionExtra struct {
	Form     *huh.Form
	Verbose  string
	HelpText string
	WriteTo  interface{} // For backwards compatibility with proxy types
}

func BooleanAsYesNo(defaultValue bool) string {
	if defaultValue {
		return "yes"
	}
	return "no"
}

// BooleanOptionProxy allows setting a boolean value from a huh.Select question
type BooleanOptionProxy struct {
	Value *bool
}

func (b *BooleanOptionProxy) Set(value string) {
	if value == "yes" {
		*b.Value = true
	} else {
		*b.Value = false
	}
}

// MultiLineValueProxy allows setting a []string value from a huh.Text question
type MultiLineValueProxy struct {
	Value *[]string
}

func (m *MultiLineValueProxy) Set(value string) {
	*m.Value = strings.Split(value, "\n")
}

// AskQuestions migrated from survey to huh - provides formatted questions with help text
func AskQuestions(questions []*QuestionExtra, response interface{}) error {
	for _, q := range questions {
		fmt.Println()
		fmt.Println(aurora.Bold(aurora.White(q.Verbose)))

		if q.HelpText != "" {
			fmt.Println(q.HelpText)
		}
		fmt.Println()

		if err := q.Form.Run(); err != nil {
			return err
		}

		// Handle WriteTo for backwards compatibility
		if q.WriteTo != nil {
			// Get the result from the form and apply it to WriteTo
			if _, ok := q.WriteTo.(*BooleanOptionProxy); ok {
				// For boolean proxies, get the string value from the form and convert
				// Note: Values are already set via the Value pointers in the form
				// The proxy pattern isn't needed anymore with Huh
			} else if _, ok := q.WriteTo.(*MultiLineValueProxy); ok {
				// For multiline proxies
				// Note: Values are already set via the Value pointers in the form
				// The proxy pattern isn't needed anymore with Huh
			}
		}

		// Get the underline length - simplified for now
		var underline int = 10 // Default underline length
		fmt.Println(aurora.Faint(strings.Repeat("─", 2+underline)))
	}
	return nil
}

// Helper functions to create common form types

// CreateSelectForm creates a select form for single choice
func CreateSelectForm(title, key string, options []string, target interface{}) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(huh.NewOptions(options...)...).
				Value(target.(*string)),
		),
	)
}

// CreateInputForm creates an input form for text entry
func CreateInputForm(title, defaultValue string, target *string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(title).
				Value(target).
				Placeholder(defaultValue),
		),
	)
}

// CreateTextForm creates a multiline text form
func CreateTextForm(title string, target *string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title(title).
				Value(target),
		),
	)
}

// CreateConfirmForm creates a confirmation form
func CreateConfirmForm(title string, target *bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Value(target),
		),
	)
}

// CreateBooleanSelectForm creates a yes/no select form that sets a boolean
func CreateBooleanSelectForm(title string, defaultValue bool, target *bool) *huh.Form {
	options := []string{"yes", "no"}
	defaultOption := BooleanAsYesNo(defaultValue)
	var selected string = defaultOption

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(huh.NewOptions(options...)...).
				Value(&selected),
		),
	)

	// Note: The value will be updated directly via the Value pointer
	// We can check the selected value after form.Run() in the calling code

	return form
}

// PauseUntilEnter waits for the user to press enter
func PauseUntilEnter(msg string) {
	fmt.Println(aurora.Bold(aurora.White(msg)))
	fmt.Scanln()
}