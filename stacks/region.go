package stacks

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/spf13/pflag"
)

type RegionStackParameters struct {
	DockerhubUsername    string `flag:"dockerhub-username"`
	DockerhubAccessToken string `flag:"dockerhub-access-token"`
}

func (p *RegionStackParameters) Import(parameters []*cloudformation.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *RegionStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	cfnParams, err := StructToCloudformationParameters(p)
	if err != nil {
		return nil, err
	}
	// pop DockerhubAccessToken from the list of parameters
	// it is stored in SSM instead of getting directly passed to CloudFormation
	accessTokenIndex := -1
	for i, param := range cfnParams {
		if *param.ParameterKey == "DockerhubAccessToken" {
			accessTokenIndex = i
			break
		}
	}
	if accessTokenIndex == -1 {
		return nil, fmt.Errorf("DockerhubAccessToken not found in parameters")
	}
	return append(cfnParams[:accessTokenIndex], cfnParams[accessTokenIndex+1:]...), nil
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *RegionStackParameters) SetInternalFields(sess *session.Session, name *string) error {
	ui.StartSpinner()
	ssmSvc := ssm.New(sess)
	_, err := ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:  aws.String("/apppack/account/dockerhub-access-token"),
		Value: &p.DockerhubAccessToken,
		Type:  aws.String("SecureString"),
		Tags: []*ssm.Tag{
			{Key: aws.String("apppack:region"), Value: name},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		},
	})
	if err != nil {
		return err
	}
	ui.Spinner.Stop()
	return nil
}

type RegionStack struct {
	Stack      *cloudformation.Stack
	Parameters *RegionStackParameters
}

func (a *RegionStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *RegionStack) GetStack() *cloudformation.Stack {
	return a.Stack
}

func (a *RegionStack) SetStack(stack *cloudformation.Stack) {
	a.Stack = stack
}

func (*RegionStack) PostCreate(_ *session.Session) error {
	return nil
}

func (*RegionStack) PreDelete(_ *session.Session) error {
	return nil
}

func (*RegionStack) PostDelete(sess *session.Session, _ *string) error {
	ssmSvc := ssm.New(sess)
	_, err := ssmSvc.DeleteParameter(&ssm.DeleteParameterInput{
		Name: aws.String("/apppack/account/dockerhub-access-token"),
	})
	return err
}

func (a *RegionStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (a *RegionStack) AskQuestions(_ *session.Session) error {
	questions := []*ui.QuestionExtra{
		{
			Verbose:  "What is your Docker Hub username?",
			HelpText: "App images will be created using base images from Docker Hub. To avoid hitting rate limits during the build process, a free Docker Hub account is required. See https://docs.docker.com/docker-hub/download-rate-limit/ for more info.",
			Question: &survey.Question{
				Name:     "DockerhubUsername",
				Prompt:   &survey.Input{Message: "Docker Hub Username", Default: a.Parameters.DockerhubUsername},
				Validate: survey.Required,
			},
		},
		{
			Verbose:  "What is your Docker Hub access token?",
			HelpText: "An access token for your Docker Hub account can be generated at https://hub.docker.com/settings/security.",
			Question: &survey.Question{
				Name:     "DockerhubAccessToken",
				Prompt:   &survey.Password{Message: "Docker Hub Access Token"},
				Validate: survey.Required,
			},
		},
	}
	return ui.AskQuestions(questions, a.Parameters)
}

func (*RegionStack) StackName(name *string) *string {
	stackName := fmt.Sprintf(regionStackNameTmpl, *name)
	return &stackName
}

func (*RegionStack) StackType() string {
	return "region"
}

func (*RegionStack) Tags(name *string) []*cloudformation.Tag {
	return []*cloudformation.Tag{
		{Key: aws.String("apppack:region"), Value: name},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*RegionStack) Capabilities() []*string {
	return []*string{
		aws.String("CAPABILITY_IAM"),
	}
}

func (*RegionStack) TemplateURL(release *string) *string {
	url := regionFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}
	return &url
}
