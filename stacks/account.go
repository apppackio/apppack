package stacks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/charmbracelet/huh"
	"github.com/spf13/pflag"
)

type AccountStackParameters struct {
	Administrators []string `flag:"administrators"`
}

func (p *AccountStackParameters) Import(parameters []types.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *AccountStackParameters) ToCloudFormationParameters() ([]types.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (*AccountStackParameters) SetInternalFields(_ aws.Config, _ *string) error {
	return nil
}

type AccountStack struct {
	Stack      *types.Stack
	Parameters *AccountStackParameters
}

func (a *AccountStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *AccountStack) GetStack() *types.Stack {
	return a.Stack
}

func (a *AccountStack) SetStack(stack *types.Stack) {
	a.Stack = stack
}

func (*AccountStack) PostCreate(_ aws.Config) error {
	return nil
}

func (*AccountStack) PreDelete(_ aws.Config) error {
	return nil
}

func (*AccountStack) PostDelete(_ aws.Config, _ *string) error {
	return nil
}

func (a *AccountStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	err := ui.FlagsToStruct(a.Parameters, flags)
	if err != nil {
		return err
	}

	sort.Strings(a.Parameters.Administrators)

	return nil
}

func (a *AccountStack) AskQuestions(_ aws.Config) error {
	form, adminsPtr := AccountAdministratorsForm(strings.Join(a.Parameters.Administrators, "\n"))
	if err := form.Run(); err != nil {
		return err
	}
	a.Parameters.Administrators = splitLines(*adminsPtr)

	return nil
}

// splitLines splits a string by newlines and filters out empty lines.
func splitLines(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// AccountAdministratorsForm builds the interactive form for entering administrator emails.
// Returns the form and a pointer to the raw multiline text value.
func AccountAdministratorsForm(defaultValue string) (*huh.Form, *string) {
	admins := defaultValue

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Who can administer this account?").
				Description("A list of email addresses (one per line) who have access to manage this AppPack account.\nThese users will be assigned a permissive IAM policy in your AWS account and should be fully trusted."),
			huh.NewText().
				Title("Administrators").
				Value(&admins).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("at least one administrator is required")
					}
					return nil
				}),
		),
	)

	return form, &admins
}

func (*AccountStack) StackName(_ *string) *string {
	stackName := accountStackName

	return &stackName
}

func (*AccountStack) StackType() string {
	return "account"
}

func (*AccountStack) Tags(_ *string) []types.Tag {
	return []types.Tag{
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*AccountStack) Capabilities() []types.Capability {
	return []types.Capability{
		types.CapabilityCapabilityIam,
	}
}

func (*AccountStack) TemplateURL(release *string) *string {
	url := accountFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}

	return &url
}
