package stacks

import (
	"fmt"
	"strings"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/spf13/pflag"
)

// appList gets a list of app names from DynamoDB
func appList(sess *session.Session) ([]string, error) {
	ddbSvc := dynamodb.New(sess)
	result, err := ddbSvc.Query(&dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id1": {S: aws.String("CLUSTERS")},
		},
	})
	if err != nil {
		return nil, err
	}
	type clusterItem struct {
		SecondaryID string `json:"secondary_id"`
	}
	var clusterItems []clusterItem
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &clusterItems)
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
func appForDomain(sess *session.Session, domain string) (*string, error) {
	appNames, err := appList(sess)
	if err != nil {
		return nil, err
	}
	for _, appName := range appNames {
		a := app.App{
			Name:    appName,
			Session: sess,
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

	return nil, fmt.Errorf("no app found for domain %s", domain)
}

func clusterStackForApp(sess *session.Session, appName string) (*string, error) {
	cfnSvc := cloudformation.New(sess)
	stacks, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
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

func (p *CustomDomainStackParameters) Import(parameters []*cloudformation.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *CustomDomainStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *CustomDomainStackParameters) SetInternalFields(sess *session.Session, name *string) error {
	ui.StartSpinner()
	p.PrimaryDomain = *name
	ui.Spinner.Suffix = " looking up app details"
	appName, err := appForDomain(sess, *name)
	if err != nil {
		return err
	}
	clusterStackName, err := clusterStackForApp(sess, *appName)
	if err != nil {
		return err
	}
	p.ClusterStackName = *clusterStackName
	ui.Spinner.Suffix = " verifying hosted zone"
	zone, err := bridge.HostedZoneForDomain(sess, *name)
	if err != nil {
		return err
	}
	if err = checkHostedZone(sess, zone); err != nil {
		return err
	}
	altDomainParams := []*string{&p.AltDomain1, &p.AltDomain2, &p.AltDomain3, &p.AltDomain4, &p.AltDomain5}
	for _, altDomain := range altDomainParams {
		if *altDomain == "" {
			continue
		}
		altZone, err := bridge.HostedZoneForDomain(sess, *altDomain)
		if err != nil {
			return err
		}
		if altZone.Id != zone.Id {
			return fmt.Errorf("alternate domain %s must be in the same hosted zone as the primary domain %s", *altDomain, *name)
		}
	}
	p.HostedZone = strings.Split(*zone.Id, "/")[2]
	ui.Spinner.Stop()
	// `*` is not allowed in certificate names
	p.CertificateName = strings.ReplaceAll(*name, "*", "wildcard")
	return nil
}

type CustomDomainStack struct {
	Stack      *cloudformation.Stack
	Parameters *CustomDomainStackParameters
}

func (a *CustomDomainStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *CustomDomainStack) GetStack() *cloudformation.Stack {
	return a.Stack
}

func (a *CustomDomainStack) SetStack(stack *cloudformation.Stack) {
	a.Stack = stack
}

func (*CustomDomainStack) PostCreate(_ *session.Session) error {
	return nil
}

func (*CustomDomainStack) PreDelete(_ *session.Session) error {
	return nil
}

func (*CustomDomainStack) PostDelete(_ *session.Session, _ *string) error {
	return nil
}

func (a *CustomDomainStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (*CustomDomainStack) AskQuestions(_ *session.Session) error {
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

func (a *CustomDomainStack) Tags(*string) []*cloudformation.Tag {
	return []*cloudformation.Tag{
		{Key: aws.String("apppack:customDomain"), Value: &a.Parameters.CertificateName},
		// TODO
		// {Key: aws.String("apppack:appName"), Value: appName},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*CustomDomainStack) Capabilities() []*string {
	return []*string{
		aws.String("CAPABILITY_IAM"),
	}
}

func (*CustomDomainStack) TemplateURL(release *string) *string {
	url := customDomainFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}
	return &url
}
