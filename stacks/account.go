package stacks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
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
	return ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  "Who can administer this account?",
			HelpText: "A list of email addresses (one per line) who have access to manage this AppPack account. These users will be assigned a permissive IAM policy in your AWS account and should be fully trusted with any resources within ",
			Question: &survey.Question{
				Name:     "Administrators",
				Prompt:   &survey.Multiline{Message: "Administrators"},
				Validate: survey.Required,
			},
		},
	}, a.Parameters)
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
