package stacks

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

var previousElasticacheGenerations = []string{
	"cache.t3",
	"cache.r5.",
	"cache.m5.",
}

func isPreviousElasticacheGeneration(instanceClass *string) bool {
	for _, p := range previousElasticacheGenerations {
		if strings.HasPrefix(*instanceClass, p) {
			return true
		}
	}

	return false
}

type RedisStackParameters struct {
	Name               string
	ClusterStackName   string `flag:"cluster;fmtString:apppack-cluster-%s"`
	InstanceClass      string `flag:"instance-class"`
	MultiAZ            bool   `cfnbool:"yesno"                             flag:"multi-az"`
	AuthTokenParameter string
}

func (p *RedisStackParameters) Import(parameters []types.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *RedisStackParameters) ToCloudFormationParameters() ([]types.Parameter, error) {
	return StructToCloudformationParameters(p)
}

var DefaultRedisStackParameters = RedisStackParameters{
	InstanceClass: "cache.t4g.micro",
	MultiAZ:       false,
}

func listElasticacheInstanceClasses(cfg aws.Config) ([]string, error) {
	elasticacheSvc := elasticache.NewFromConfig(cfg)

	out, err := elasticacheSvc.DescribeReservedCacheNodesOfferings(context.Background(), &elasticache.DescribeReservedCacheNodesOfferingsInput{
		OfferingType:       aws.String("No Upfront"),
		Duration:           aws.String("1"),
		ProductDescription: aws.String("redis"),
	})
	if err != nil {
		return nil, err
	}

	var instanceClasses []string

	for _, opt := range out.ReservedCacheNodesOfferings {
		if !isPreviousElasticacheGeneration(opt.CacheNodeType) {
			instanceClasses = append(instanceClasses, *opt.CacheNodeType)
		}
	}

	instanceClasses = dedupe(instanceClasses)
	bridge.SortInstanceClasses(instanceClasses)

	return instanceClasses, nil
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *RedisStackParameters) SetInternalFields(cfg aws.Config, name *string) error {
	if p.AuthTokenParameter == "" {
		authToken := fmt.Sprintf(redisAuthTokenParameterTmpl, *name)
		p.AuthTokenParameter = authToken

		password, err := GeneratePassword()
		if err != nil {
			return err
		}

		ssmSvc := ssm.NewFromConfig(cfg)
		_, err = ssmSvc.PutParameter(context.Background(), &ssm.PutParameterInput{
			Name:  &authToken,
			Value: &password,
			Type:  ssmtypes.ParameterTypeSecureString,
		})

		return err
	}

	if p.Name == "" {
		p.Name = *name
	}

	return nil
}

type RedisStack struct {
	Stack      *types.Stack
	Parameters *RedisStackParameters
}

func (a *RedisStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *RedisStack) GetStack() *types.Stack {
	return a.Stack
}

func (a *RedisStack) SetStack(stack *types.Stack) {
	a.Stack = stack
}

func (*RedisStack) PostCreate(_ aws.Config) error {
	return nil
}

func (*RedisStack) PreDelete(_ aws.Config) error {
	return nil
}

func (a *RedisStack) PostDelete(cfg aws.Config, name *string) error {
	// PostDelete gets called during destroy even if the stack doesn't exist
	// to cleanup orphaned resources. In that scenario, the name is provided
	// otherwise it can be looked up from the Stack.
	if name == nil {
		name = aws.String("")

		_, err := fmt.Sscanf(*a.Stack.StackName, redisStackNameTmpl, name)
		if err != nil {
			return err
		}
	}

	parameterName := fmt.Sprintf(redisAuthTokenParameterTmpl, *name)
	logrus.WithFields(logrus.Fields{"name": parameterName}).Debug("deleting SSM parameter")

	ssmSvc := ssm.NewFromConfig(cfg)
	_, err := ssmSvc.DeleteParameter(context.Background(), &ssm.DeleteParameterInput{
		Name: &parameterName,
	})

	return err
}

func (a *RedisStack) ClusterName() string {
	return strings.TrimPrefix(a.Parameters.ClusterStackName, fmt.Sprintf(clusterStackNameTmpl, ""))
}

func (a *RedisStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (a *RedisStack) AskQuestions(cfg aws.Config) error {
	var questions []*ui.QuestionExtra

	var err error
	if a.Stack == nil {
		err = AskForCluster(
			cfg,
			"Which cluster should this Redis instance be installed in?",
			"A cluster represents an isolated network and its associated resources (Apps, Database, Redis, etc.).",
			a.Parameters,
		)
		if err != nil {
			return err
		}
	}

	if a.Parameters.InstanceClass == "" {
		a.Parameters.InstanceClass = DefaultRedisStackParameters.InstanceClass
	}

	questions = append(questions, []*ui.QuestionExtra{
		{
			Verbose: "Should this Redis instance be setup in multiple availability zones?",
			HelpText: "Multiple availability zones (AZs) provide more resilience in the case of an AZ outage, " +
				"but double the cost at AWS. For more info see " +
				"https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html.",
			WriteTo: &ui.BooleanOptionProxy{Value: &a.Parameters.MultiAZ},
			Question: &survey.Question{
				Prompt: &survey.Select{
					Message:       "Multi AZ",
					Options:       []string{"yes", "no"},
					FilterMessage: "",
					Default:       ui.BooleanAsYesNo(a.Parameters.MultiAZ),
				},
			},
		},
	}...)
	if err = ui.AskQuestions(questions, a.Parameters); err != nil {
		return err
	}
	// Clear the questions slice so we can reuse it
	questions = questions[:0]

	ui.StartSpinner()
	ui.Spinner.Suffix = " retrieving instance classes"

	instanceClasses, err := listElasticacheInstanceClasses(cfg)
	if err != nil {
		return err
	}

	ui.Spinner.Stop()
	ui.Spinner.Suffix = ""

	questions = append(questions, []*ui.QuestionExtra{
		{
			Verbose:  "What instance class should be used for this Redis instance?",
			HelpText: "Enter the Redis instance class. For more info see https://aws.amazon.com/elasticache/pricing/.",
			Question: &survey.Question{
				Name: "InstanceClass",
				Prompt: &survey.Select{
					Message:       "Instance Class",
					Options:       instanceClasses,
					FilterMessage: "",
					Default:       a.Parameters.InstanceClass,
				},
				Validate: survey.Required,
			},
		},
	}...)

	return ui.AskQuestions(questions, a.Parameters)
}

func (*RedisStack) StackName(name *string) *string {
	stackName := fmt.Sprintf(redisStackNameTmpl, *name)

	return &stackName
}

func (*RedisStack) StackType() string {
	return "redis"
}

func (a *RedisStack) Tags(name *string) []types.Tag {
	return []types.Tag{
		{Key: aws.String("apppack:redis"), Value: name},
		{Key: aws.String("apppack:cluster"), Value: aws.String(a.ClusterName())},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*RedisStack) Capabilities() []types.Capability {
	return []types.Capability{
		types.CapabilityCapabilityIam,
	}
}

func (*RedisStack) TemplateURL(release *string) *string {
	url := redisFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}

	return &url
}
