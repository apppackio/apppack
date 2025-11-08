package ui

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/core"
	"github.com/logrusorgru/aurora"
)

type QuestionExtra struct {
	Question *survey.Question
	Verbose  string
	HelpText string
	WriteTo  core.Settable
}

func BooleanAsYesNo(defaultValue bool) string {
	if defaultValue {
		return "yes"
	}
	return "no"
}

// BooleanOptionProxy allows setting a boolean value from a survey.Select question
type BooleanOptionProxy struct {
	Value *bool
}

func (b *BooleanOptionProxy) WriteAnswer(_ string, value interface{}) error {
	ans, ok := value.(core.OptionAnswer)
	if !ok {
		return fmt.Errorf("unable to convert value to OptionAnswer")
	}

	if ans.Value == "yes" {
		*b.Value = true
	} else {
		*b.Value = false
	}
	return nil
}

// MultiLineValueProxy allows setting a []string value from a survey.Multiline question
type MultiLineValueProxy struct {
	Value *[]string
}

func (m *MultiLineValueProxy) WriteAnswer(_ string, value interface{}) error {
	ans, ok := value.(string)
	if !ok {
		return fmt.Errorf("unable to convert value to string")
	}
	*m.Value = strings.Split(ans, "\n")
	return nil
}

// AskQuestions tweaks survey.Ask (and AskOne) to format things the way we want
func AskQuestions(questions []*QuestionExtra, response interface{}) error {
	for _, q := range questions {
		fmt.Println()
		fmt.Println(aurora.Bold(aurora.White(q.Verbose)))

		if q.HelpText != "" {
			fmt.Println(q.HelpText)
		}

		fmt.Println()

		if q.WriteTo == nil {
			if err := survey.Ask([]*survey.Question{q.Question}, response, survey.WithShowCursor(true)); err != nil {
				return err
			}
		} else {
			if q.Question.Validate != nil {
				if err := survey.AskOne(q.Question.Prompt, q.WriteTo, survey.WithShowCursor(true), survey.WithValidator(q.Question.Validate)); err != nil {
					return err
				}
			} else {
				if err := survey.AskOne(q.Question.Prompt, q.WriteTo, survey.WithShowCursor(true)); err != nil {
					return err
				}
			}
		}

		var underline int
		if p, ok := q.Question.Prompt.(*survey.Input); ok {
			underline = len(p.Message)
		} else if p, ok := q.Question.Prompt.(*survey.Select); ok {
			underline = len(p.Message)
		} else if p, ok := q.Question.Prompt.(*survey.Multiline); ok {
			underline = len(p.Message)
		} else if p, ok := q.Question.Prompt.(*survey.Password); ok {
			underline = len(p.Message)
		}

		fmt.Println(aurora.Faint(strings.Repeat("─", 2+underline)))
	}
	return nil
}

// PauseUntilEnter waits for the user to press enter
func PauseUntilEnter(msg string) {
	fmt.Println(aurora.Bold(aurora.White(msg)))
	_, _ = fmt.Scanln()
}
