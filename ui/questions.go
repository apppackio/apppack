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

// YesNoOptions returns huh options for a boolean yes/no select.
//
// It deliberately does not mark either option `.Selected(true)`: every
// caller binds the field's value to a string pre-seeded via
// BooleanAsYesNo(defaultValue), and huh.Select positions its cursor on the
// option whose Value matches the bound value. Marking an option
// `.Selected(true)` as well is redundant, and it also triggers a huh bug
// (see https://github.com/apppackio/apppack/issues/181): huh's initial
// viewport offset is derived from whichever option matched first, whether
// that match came from Value or Selected, and it isn't clamped, so a match
// at a non-zero index scrolls earlier options off the top of the list on
// first render. Seeding the bound value alone avoids that code path.
func YesNoOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("yes", "yes"),
		huh.NewOption("no", "no"),
	}
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
