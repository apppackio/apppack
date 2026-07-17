package stacks

import (
	"fmt"
	"sort"
	"strings"

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

// AskQuestions is a no-op for AccountStack — administrators are managed
// via the `admins add/remove` commands which take email args directly.
func (*AccountStack) AskQuestions(_ aws.Config) error {
	return nil
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
