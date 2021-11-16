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

type RedisStackParameters struct {
	Name               string
	ClusterStackName   string `flag:"cluster;fmtString:apppack-cluster-%s"`
	InstanceClass      string `flag:"instance-lass"`
	MultiAZ            bool   `flag:"multi-az"`
	AuthTokenParameter string
}

func (p *RedisStackParameters) Import(parameters []*cloudformation.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *RedisStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *RedisStackParameters) SetInternalFields(sess *session.Session, name *string) error {
	if p.AuthTokenParameter == "" {
		authToken := fmt.Sprintf(redisAuthTokenParameterTmpl, name)
		p.AuthTokenParameter = authToken
		ssmSvc := ssm.New(sess)
		_, err := ssmSvc.PutParameter(&ssm.PutParameterInput{
			Name:  &authToken,
			Value: aws.String(generatePassword()),
			Type:  aws.String("SecureString"),
		})
		return err
	}
	return nil
}

type RedisStack struct {
	Stack      *cloudformation.Stack
	Parameters *RedisStackParameters
}

func (a *RedisStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *RedisStack) GetStack() *cloudformation.Stack {
	return a.Stack
}

func (a *RedisStack) SetStack(stack *cloudformation.Stack) {
	a.Stack = stack
}

func (a *RedisStack) ClusterName() string {
	return strings.TrimPrefix(a.Parameters.ClusterStackName, fmt.Sprintf(clusterStackNameTmpl, ""))
}

func (a *RedisStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (a *RedisStack) AskQuestions(sess *session.Session) error {
	questions := []*ui.QuestionExtra{}
	var err error
	if a.Stack == nil {
		err = AskForCluster(
			sess,
			"Which cluster should this Redis instance be installed in?",
			"A cluster represents an isolated network and its associated resources (Apps, Database, Redis, etc.).",
			a.Parameters,
		)
		if err != nil {
			return err
		}
	}
	if a.Parameters.InstanceClass == "" {
		a.Parameters.InstanceClass = "cache.t3.micro"
	}
	questions = append(questions, []*ui.QuestionExtra{
		{
			Verbose:  "Should this Redis instance be setup in multiple availability zones?",
			HelpText: "Multiple availability zones (AZs) provide more resilience in the case of an AZ outage, but double the cost at AWS. For more info see https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html.",
			WriteTo:  &ui.BooleanOptionProxy{Value: &a.Parameters.MultiAZ},
			Question: &survey.Question{
				Prompt: &survey.Select{
					Message:       "Multi AZ",
					Options:       []string{"yes", "no"},
					FilterMessage: "",
					Default:       ui.BooleanAsYesNo(a.Parameters.MultiAZ),
				},
			},
		},
		{
			Verbose:  "What instance class should be used for this Redis instance?",
			HelpText: "Enter the Redis instance class. For more info see https://aws.amazon.com/elasticache/pricing/.",
			Question: &survey.Question{
				Name:     "InstanceClass",
				Prompt:   &survey.Input{Message: "InstanceClass", Default: a.Parameters.InstanceClass},
				Validate: survey.Required,
			},
		},
	}...)
	if err = ui.AskQuestions(questions, a.Parameters); err != nil {
		return err
	}
	return nil
}


func (a *RedisStack) StackName(name *string) *string {
	stackName := fmt.Sprintf(redisStackNameTmpl, *name)
	return &stackName
}

func (a *RedisStack) Tags(name *string) []*cloudformation.Tag {
	return []*cloudformation.Tag{
		{Key: aws.String("apppack:redis"), Value: name},
		{Key: aws.String("apppack:cluster"), Value: aws.String(a.ClusterName())},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (a *RedisStack) Capabilities() []*string {
	return []*string{
		aws.String("CAPABILITY_IAM"),
	}
}

func (a *RedisStack) TemplateURL(release *string) *string {
	url := redisFormationURL
	if release != nil {
		url = strings.Replace(appFormationURL, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}
	return &url
}
