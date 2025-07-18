package stacks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/ssm"
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
	InstanceClass      string `flag:"instance-lass"`
	MultiAZ            bool   `flag:"multi-az" cfnbool:"yesno"`
	AuthTokenParameter string
}

func (p *RedisStackParameters) Import(parameters []*cloudformation.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *RedisStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	return StructToCloudformationParameters(p)
}

var DefaultRedisStackParameters = RedisStackParameters{
	InstanceClass: "cache.t4g.micro",
	MultiAZ:       false,
}

func listElasticacheInstanceClasses(sess *session.Session) ([]string, error) {
	elasticacheSvc := elasticache.New(sess)

	out, err := elasticacheSvc.DescribeReservedCacheNodesOfferings(&elasticache.DescribeReservedCacheNodesOfferingsInput{
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
func (p *RedisStackParameters) SetInternalFields(sess *session.Session, name *string) error {
	if p.AuthTokenParameter == "" {
		authToken := fmt.Sprintf(redisAuthTokenParameterTmpl, *name)
		p.AuthTokenParameter = authToken
		password, err := GeneratePassword()
		if err != nil {
			return err
		}
		ssmSvc := ssm.New(sess)
		_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
			Name:  &authToken,
			Value: &password,
			Type:  aws.String("SecureString"),
		})
		return err
	}
	if p.Name == "" {
		p.Name = *name
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

func (*RedisStack) PostCreate(_ *session.Session) error {
	return nil
}

func (*RedisStack) PreDelete(_ *session.Session) error {
	return nil
}

func (a *RedisStack) PostDelete(sess *session.Session, name *string) error {
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
	ssmSvc := ssm.New(sess)
	_, err := ssmSvc.DeleteParameter(&ssm.DeleteParameterInput{
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

func (a *RedisStack) AskQuestions(sess *session.Session) error {
	var questions []*ui.QuestionExtra
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
		a.Parameters.InstanceClass = DefaultRedisStackParameters.InstanceClass
	}

	var multiAZSel = ui.BooleanAsYesNo(a.Parameters.MultiAZ)
	questions = append(questions, []*ui.QuestionExtra{
		{
			Verbose: "Should this Redis instance be setup in multiple availability zones?",
			HelpText: "Multiple availability zones (AZs) provide more resilience in the case of an AZ outage, " +
				"but double the cost at AWS. For more info see " +
				"https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html.",
			WriteTo: &ui.BooleanOptionProxy{Value: &a.Parameters.MultiAZ},
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Multi AZ").
						Options(huh.NewOptions("yes", "no")...).
						Value(&multiAZSel),
				),
			),
		},
	}...)
	if err = ui.AskQuestions(questions, a.Parameters); err != nil {
		return err
	}
	// Clear the questions slice so we can reuse it
	questions = questions[:0]

	ui.StartSpinner()
	ui.Spinner.Suffix = " retrieving instance classes"
	instanceClasses, err := listElasticacheInstanceClasses(sess)
	if err != nil {
		return err
	}
	ui.Spinner.Stop()
	ui.Spinner.Suffix = ""

	questions = append(questions, []*ui.QuestionExtra{
		{
			Verbose:  "What instance class should be used for this Redis instance?",
			HelpText: "Enter the Redis instance class. For more info see https://aws.amazon.com/elasticache/pricing/.",
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Instance Class").
						Options(huh.NewOptions(instanceClasses...)...).
						Value(&a.Parameters.InstanceClass),
				),
			),
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

func (a *RedisStack) Tags(name *string) []*cloudformation.Tag {
	return []*cloudformation.Tag{
		{Key: aws.String("apppack:redis"), Value: name},
		{Key: aws.String("apppack:cluster"), Value: aws.String(a.ClusterName())},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*RedisStack) Capabilities() []*string {
	return []*string{
		aws.String("CAPABILITY_IAM"),
	}
}

func (*RedisStack) TemplateURL(release *string) *string {
	url := redisFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}
	return &url
}
