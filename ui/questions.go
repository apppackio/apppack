package ui

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/logrusorgru/aurora"
)

func BooleanAsYesNo(defaultValue bool) string {
	if defaultValue {
		return "yes"
	}

	return "no"
}

// PauseUntilEnter waits for the user to press enter
func PauseUntilEnter(msg string) {
	fmt.Println(aurora.Bold(aurora.White(msg)))
	_, _ = fmt.Scanln()
}

// YesNoOptions returns huh options for a boolean yes/no select, with the
// given default pre-selected.
func YesNoOptions(defaultValue bool) []huh.Option[string] {
	opts := []huh.Option[string]{
		huh.NewOption("yes", "yes"),
		huh.NewOption("no", "no"),
	}
	if defaultValue {
		opts[0] = opts[0].Selected(true)
	} else {
		opts[1] = opts[1].Selected(true)
	}

	return opts
}

// YesNoToBool converts a "yes"/"no" string to a boolean.
func YesNoToBool(val string) bool {
	return val == "yes"
}

// PrintQuestionHeader prints the verbose title and optional help text for a
// question, matching the existing AskQuestions visual style.
func PrintQuestionHeader(verbose, helpText string) {
	fmt.Println()
	fmt.Println(aurora.Bold(aurora.White(verbose)))

	if helpText != "" {
		fmt.Println(helpText)
	}

	fmt.Println()
}
