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
}

func BooleanAsYesNo(defaultValue bool) string {
	if defaultValue {
		return "yes"
	}
	return "no"
}


// AskQuestions migrated from survey to huh - provides formatted questions with help text
func AskQuestions(questions []*QuestionExtra, _ interface{}) error {
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

		// Forms directly update their target variables via Value() pointers

		// Get the underline length - simplified for now
		var underline = 10 // Default underline length
		fmt.Println(aurora.Faint(strings.Repeat("─", 2+underline)))
	}
	return nil
}

// CreateSelectForm creates a select form for single choice
func CreateSelectForm(title, _ string, options []string, target interface{}) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(huh.NewOptions(options...)...).
				Value(target.(*string)),
		),
	)
}

// PauseUntilEnter waits for the user to press enter
func PauseUntilEnter(msg string) {
	fmt.Println(aurora.Bold(aurora.White(msg)))
	fmt.Scanln()
}