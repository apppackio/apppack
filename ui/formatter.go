package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
)

// Spinner is the loading animation to use for all commands
var Spinner *spinner.Spinner = spinner.New(spinner.CharSets[14], 50*time.Millisecond)

func StartSpinner() {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		Spinner.Start()
	}
}

func PrintError(text string) {
	fmt.Println(aurora.Red(fmt.Sprintf("✖ %s", text)))
}

func PrintSuccess(text string) {
	fmt.Println(aurora.Green(fmt.Sprintf("✔ %s", text)))
}

func PrintWarning(text string) {
	fmt.Println(aurora.Yellow(fmt.Sprintf("⚠  %s", text)))
}