package stacks

import (
	"context"
	"fmt"
	"strings"

	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

type RegionStackParameters struct{}

func (p *RegionStackParameters) Import(parameters []types.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *RegionStackParameters) ToCloudFormationParameters() ([]types.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (*RegionStackParameters) SetInternalFields(_ aws.Config, _ *string) error {
	return nil
}

type RegionStack struct {
	Stack      *types.Stack
	Parameters *RegionStackParameters
}

func (a *RegionStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *RegionStack) GetStack() *types.Stack {
	return a.Stack
}

func (a *RegionStack) SetStack(stack *types.Stack) {
	a.Stack = stack
}

func (*RegionStack) PostCreate(_ aws.Config) error {
	return nil
}

func (*RegionStack) PreDelete(_ aws.Config) error {
	return nil
}

func (*RegionStack) PostDelete(cfg aws.Config, _ *string) error {
	// Stacks before `formations/5.8.0` used this parameter
	ssmSvc := ssm.NewFromConfig(cfg)
	_, err := ssmSvc.DeleteParameter(context.Background(), &ssm.DeleteParameterInput{
		Name: aws.String("/apppack/account/dockerhub-access-token"),
	})
	// Ignore error if the parameter doesn't exist
	if err != nil && strings.Contains(err.Error(), "ParameterNotFound") {
		logrus.WithError(err).Debug("dockerhub-access-token parameter does not exist")

		return nil
	}

	return err
}

func (a *RegionStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (*RegionStack) AskQuestions(_ aws.Config) error {
	return nil
}

func (*RegionStack) StackName(name *string) *string {
	stackName := fmt.Sprintf(regionStackNameTmpl, *name)

	return &stackName
}

func (*RegionStack) StackType() string {
	return "region"
}

func (*RegionStack) Tags(name *string) []types.Tag {
	return []types.Tag{
		{Key: aws.String("apppack:region"), Value: name},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*RegionStack) Capabilities() []types.Capability {
	return []types.Capability{
		types.CapabilityCapabilityIam,
	}
}

func (*RegionStack) TemplateURL(release *string) *string {
	url := regionFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}

	return &url
}
