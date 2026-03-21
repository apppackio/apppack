package stacks

import (
	"context"
	"fmt"
	"strings"

	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/charmbracelet/huh"
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
	if a.Stack == nil {
		err := AskForCluster(
			cfg,
			"Which cluster should this Redis instance be installed in?",
			"A cluster represents an isolated network and its associated resources (Apps, Database, Redis, etc.).",
			&a.Parameters.ClusterStackName,
		)
		if err != nil {
			return err
		}
	}

	if a.Parameters.InstanceClass == "" {
		a.Parameters.InstanceClass = DefaultRedisStackParameters.InstanceClass
	}

	// Multi-AZ prompt
	multiAZForm, multiAZPtr := RedisMultiAZForm(a.Parameters.MultiAZ)
	if err := multiAZForm.Run(); err != nil {
		return err
	}
	a.Parameters.MultiAZ = ui.YesNoToBool(*multiAZPtr)

	// Fetch instance classes from AWS
	ui.StartSpinner()
	ui.Spinner.Suffix = " retrieving instance classes"

	instanceClasses, err := listElasticacheInstanceClasses(cfg)
	if err != nil {
		return err
	}

	ui.Spinner.Stop()
	ui.Spinner.Suffix = ""

	// Instance class prompt
	instanceClassForm, instanceClassPtr := RedisInstanceClassForm(instanceClasses, a.Parameters.InstanceClass)
	if err := instanceClassForm.Run(); err != nil {
		return err
	}
	a.Parameters.InstanceClass = *instanceClassPtr

	return nil
}

// RedisMultiAZForm builds the interactive form for selecting multi-AZ mode.
// Returns the form and a pointer to the selected "yes"/"no" value.
func RedisMultiAZForm(defaultMultiAZ bool) (*huh.Form, *string) {
	selected := ui.BooleanAsYesNo(defaultMultiAZ)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Should this Redis instance be setup in multiple availability zones?").
				Description("Multiple availability zones (AZs) provide more resilience in the case of an AZ outage,\nbut double the cost at AWS. For more info see\nhttps://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html."),
			huh.NewSelect[string]().
				Title("Multi AZ").
				Options(ui.YesNoOptions(defaultMultiAZ)...).
				Value(&selected),
		),
	)

	return form, &selected
}

// RedisInstanceClassForm builds the interactive form for selecting an instance class.
// Returns the form and a pointer to the selected instance class.
func RedisInstanceClassForm(instanceClasses []string, defaultClass string) (*huh.Form, *string) {
	selected := defaultClass

	options := make([]huh.Option[string], len(instanceClasses))
	for i, c := range instanceClasses {
		opt := huh.NewOption(c, c)
		if c == defaultClass {
			opt = opt.Selected(true)
		}
		options[i] = opt
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("What instance class should be used for this Redis instance?").
				Description("Enter the Redis instance class. For more info see https://aws.amazon.com/elasticache/pricing/."),
			huh.NewSelect[string]().
				Title("Instance Class").
				Options(options...).
				Value(&selected),
		),
	)

	return form, &selected
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
