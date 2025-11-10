package stacks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/spf13/pflag"
)

var (
	ErrAltDomainNotInSameHostedZone = errors.New("alternate domain must be in the same hosted zone as the primary domain")
	ErrAppNotFound                  = errors.New("app not found for domain")
)

// appList gets a list of app names from DynamoDB
func appList(cfg aws.Config) ([]string, error) {
	ddbSvc := dynamodb.NewFromConfig(cfg)

	result, err := ddbSvc.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":id1": &ddbtypes.AttributeValueMemberS{Value: "CLUSTERS"},
		},
	})
	if err != nil {
		return nil, err
	}

	type clusterItem struct {
		SecondaryID string `dynamodbav:"secondary_id"`
	}

	var clusterItems []clusterItem

	err = attributevalue.UnmarshalListOfMaps(result.Items, &clusterItems)
	if err != nil {
		return nil, err
	}

	var appNames []string

	for _, item := range clusterItems {
		if strings.Contains(item.SecondaryID, "#APP#") {
			appNames = append(appNames, strings.Split(item.SecondaryID, "#")[2])
		}
	}

	return appNames, nil
}

// appForDomain returns the app name associated with a given domain
func appForDomain(cfg aws.Config, domain string) (*string, error) {
	appNames, err := appList(cfg)
	if err != nil {
		return nil, err
	}

	for _, appName := range appNames {
		a := app.App{
			Name:    appName,
			Session: cfg,
		}
		if err = a.LoadSettings(); err != nil {
			return nil, err
		}

		for _, appDomain := range a.Settings.Domains {
			if appDomain == domain {
				return &appName, nil
			}
		}
	}

	return nil, ErrAppNotFound
}

func clusterStackForApp(cfg aws.Config, appName string) (*string, error) {
	cfnSvc := cloudformation.NewFromConfig(cfg)

	stacks, err := cfnSvc.DescribeStacks(context.Background(), &cloudformation.DescribeStacksInput{
		StackName: aws.String(fmt.Sprintf(AppStackNameTmpl, appName)),
	})
	if err != nil {
		return nil, err
	}

	for _, parameter := range stacks.Stacks[0].Parameters {
		if *parameter.ParameterKey == "ClusterStackName" {
			return parameter.ParameterValue, nil
		}
	}

	return nil, fmt.Errorf("app stack %s missing ClusterStackName parameter", appName)
}

type CustomDomainStackParameters struct {
	HostedZone       string
	ClusterStackName string
	PrimaryDomain    string
	CertificateName  string
	AltDomain1       string
	AltDomain2       string
	AltDomain3       string
	AltDomain4       string
	AltDomain5       string
}

func (p *CustomDomainStackParameters) Import(parameters []types.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *CustomDomainStackParameters) ToCloudFormationParameters() ([]types.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *CustomDomainStackParameters) SetInternalFields(cfg aws.Config, name *string) error {
	ui.StartSpinner()

	p.PrimaryDomain = *name
	ui.Spinner.Suffix = " looking up app details"

	appName, err := appForDomain(cfg, *name)
	if err != nil {
		return err
	}

	clusterStackName, err := clusterStackForApp(cfg, *appName)
	if err != nil {
		return err
	}

	p.ClusterStackName = *clusterStackName
	ui.Spinner.Suffix = " verifying hosted zone"

	zone, err := bridge.HostedZoneForDomain(cfg, *name)
	if err != nil {
		return err
	}

	if err = checkHostedZone(cfg, zone); err != nil {
		return err
	}

	altDomainParams := []*string{&p.AltDomain1, &p.AltDomain2, &p.AltDomain3, &p.AltDomain4, &p.AltDomain5}
	for _, altDomain := range altDomainParams {
		if *altDomain == "" {
			continue
		}

		altZone, err := bridge.HostedZoneForDomain(cfg, *altDomain)
		if err != nil {
			return err
		}

		if altZone.Id != zone.Id {
			return ErrAltDomainNotInSameHostedZone
		}
	}

	p.HostedZone = strings.Split(*zone.Id, "/")[2]

	ui.Spinner.Stop()
	// `*` is not allowed in certificate names
	p.CertificateName = strings.ReplaceAll(*name, "*", "wildcard")

	return nil
}

type CustomDomainStack struct {
	Stack      *types.Stack
	Parameters *CustomDomainStackParameters
}

func (a *CustomDomainStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *CustomDomainStack) GetStack() *types.Stack {
	return a.Stack
}

func (a *CustomDomainStack) SetStack(stack *types.Stack) {
	a.Stack = stack
}

func (*CustomDomainStack) PostCreate(_ aws.Config) error {
	return nil
}

func (*CustomDomainStack) PreDelete(_ aws.Config) error {
	return nil
}

func (*CustomDomainStack) PostDelete(_ aws.Config, _ *string) error {
	return nil
}

func (a *CustomDomainStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (*CustomDomainStack) AskQuestions(_ aws.Config) error {
	return nil
}

func (*CustomDomainStack) StackName(name *string) *string {
	slug := strings.ReplaceAll(strings.TrimSuffix(*name, "."), ".", "-")
	slug = strings.ReplaceAll(slug, "*", "wildcard")
	stackName := fmt.Sprintf(customDomainStackNameTmpl, slug)

	return &stackName
}

func (*CustomDomainStack) StackType() string {
	return "custom domain"
}

func (a *CustomDomainStack) Tags(*string) []types.Tag {
	return []types.Tag{
		{Key: aws.String("apppack:customDomain"), Value: &a.Parameters.CertificateName},
		// TODO: add app name tag
		// {Key: aws.String("apppack:appName"), Value: appName},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*CustomDomainStack) Capabilities() []types.Capability {
	return []types.Capability{
		types.CapabilityCapabilityIam,
	}
}

func (*CustomDomainStack) TemplateURL(release *string) *string {
	url := customDomainFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}

	return &url
}
