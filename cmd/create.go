/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apppackio/apppack/auth"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	appFormationURL             = "https://s3.amazonaws.com/apppack-cloudformations/latest/app.json"
	clusterFormationURL         = "https://s3.amazonaws.com/apppack-cloudformations/latest/cluster.json"
	accountFormationURL         = "https://s3.amazonaws.com/apppack-cloudformations/latest/account.json"
	regionFormationURL          = "https://s3.amazonaws.com/apppack-cloudformations/latest/region.json"
	postgresFormationURL        = "https://s3.amazonaws.com/apppack-cloudformations/latest/postgres.json"
	mysqlFormationURL           = "https://s3.amazonaws.com/apppack-cloudformations/latest/mysql.json"
	redisFormationURL           = "https://s3.amazonaws.com/apppack-cloudformations/latest/redis.json"
	customDomainFormationURL    = "https://s3.amazonaws.com/apppack-cloudformations/latest/custom-domain.json"
	redisStackNameTmpl          = "apppack-redis-%s"
	redisAuthTokenParameterTmpl = "/apppack/redis/%s/auth-token"
	databaseStackNameTmpl       = "apppack-database-%s"
)

var createChangeSet bool
var nonInteractive bool
var region string

func appStackName(appName string) string {
	return fmt.Sprintf("apppack-app-%s", appName)
}

func createStackOrChangeSet(sess *session.Session, input *cloudformation.CreateStackInput, changeSet bool, friendlyName string) error {
	var statusURL string
	if changeSet {
		fmt.Printf("Creating Cloudformation Change Set for %s...\n", friendlyName)
		startSpinner()
		changeSet, err := createChangeSetAndWait(sess, input)
		Spinner.Stop()
		if err != nil {
			return err
		}
		statusURL = fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", *sess.Config.Region, url.QueryEscape(*changeSet.ChangeSetId))
		if *changeSet.Status != "CREATE_COMPLETE" {
			return fmt.Errorf("Stack ChangeSet creation Failed.\nView status at %s", statusURL)
		}
		fmt.Println("View ChangeSet at:")
		fmt.Println(aurora.White(statusURL))
	} else {
		fmt.Printf("Creating %s resources...\n", friendlyName)
		startSpinner()
		stack, err := createStackAndWait(sess, input)
		Spinner.Stop()
		if err != nil {
			return err
		}
		statusURL := fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", *sess.Config.Region, url.QueryEscape(*stack.StackId))
		if *stack.StackStatus != "CREATE_COMPLETE" {
			return fmt.Errorf("Stack creation Failed.\nView status at %s", statusURL)
		}
		printSuccess(fmt.Sprintf("created %s", friendlyName))
	}
	return nil
}

func createChangeSetAndWait(sess *session.Session, stackInput *cloudformation.CreateStackInput) (*cloudformation.DescribeChangeSetOutput, error) {
	cfnSvc := cloudformation.New(sess)
	changeSetName := fmt.Sprintf("create-%d", int32(time.Now().Unix()))
	_, err := cfnSvc.CreateChangeSet(&cloudformation.CreateChangeSetInput{
		ChangeSetType: aws.String("CREATE"),
		ChangeSetName: &changeSetName,
		StackName:     stackInput.StackName,
		TemplateURL:   stackInput.TemplateURL,
		Parameters:    stackInput.Parameters,
		Capabilities:  stackInput.Capabilities,
		Tags:          stackInput.Tags,
	})
	if err != nil {
		return nil, err
	}
	describeChangeSetInput := cloudformation.DescribeChangeSetInput{
		ChangeSetName: &changeSetName,
		StackName:     stackInput.StackName,
	}
	err = cfnSvc.WaitUntilChangeSetCreateComplete(&describeChangeSetInput)
	if err != nil {
		return nil, err
	}
	changeSet, err := cfnSvc.DescribeChangeSet(&describeChangeSetInput)
	if err != nil {
		return nil, err
	}
	return changeSet, nil
}

func createStackAndWait(sess *session.Session, stackInput *cloudformation.CreateStackInput) (*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(sess)
	stackOutput, err := cfnSvc.CreateStack(stackInput)
	if err != nil {
		return nil, err
	}
	Spinner.Stop()
	fmt.Println(aurora.Faint(fmt.Sprintf("creating %s", *stackOutput.StackId)))
	return waitForCloudformationStack(cfnSvc, *stackInput.StackName)
}

// awsSession starts a session, verifying a region has been provided
func awsSession() (*session.Session, error) {
	if region != "" {
		return session.NewSession(&aws.Config{Region: &region})
	}
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}
	if *sess.Config.Region == "" {
		return nil, fmt.Errorf("no region provided. Use the `--region` flag or set the AWS_REGION environment")
	}
	return sess, err

}

type stackItem struct {
	PrimaryID   string `json:"primary_id"`
	SecondaryID string `json:"secondary_id"`
	Stack       Stack  `json:"value"`
}

type Stack struct {
	StackID        string `json:"stack_id"`
	StackName      string `json:"stack_name"`
	Name           string `json:"name"`
	DatabaseEngine string `json:"engine"`
}

func listClusters(sess *session.Session) ([]string, error) {
	ddbSvc := dynamodb.New(sess)
	result, err := ddbSvc.Query(&dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1 AND begins_with(secondary_id,:id2)"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id1": {S: aws.String("CLUSTERS")},
			":id2": {S: aws.String("CLUSTER#")},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Items == nil {
		return nil, fmt.Errorf("could not find any AppPack clusters")
	}
	i := []stackItem{}
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &i)
	if err != nil {
		return nil, err
	}
	clusters := []string{}
	for idx := range i {
		clusters = append(clusters, i[idx].Stack.Name)
	}

	return clusters, nil
}

func makeClusterQuestion(sess *session.Session, message *string) (*survey.Question, error) {
	clusters, err := listClusters(sess)
	if err != nil {
		return nil, err
	}
	if len(clusters) == 0 {
		return nil, fmt.Errorf("no AppPack clusters are setup")
	}
	var defaultCluster string
	if contains(clusters, "apppack") {
		defaultCluster = "apppack"
	} else {
		defaultCluster = clusters[0]
	}
	return &survey.Question{
		Name: "cluster",
		Prompt: &survey.Select{
			Message: *message,
			Options: clusters,
			Default: defaultCluster,
		},
	}, err
}

func getDDBClusterItem(sess *session.Session, cluster *string, addon string, name *string) (*Stack, error) {
	ddbSvc := dynamodb.New(sess)
	secondaryID := fmt.Sprintf("%s#%s#%s", *cluster, addon, *name)
	result, err := ddbSvc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("apppack"),
		Key: map[string]*dynamodb.AttributeValue{
			"primary_id": {
				S: aws.String("CLUSTERS"),
			},
			"secondary_id": {
				S: aws.String(secondaryID),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("could not find CLUSTERS/%s", secondaryID)
	}
	i := stackItem{}
	err = dynamodbattribute.UnmarshalMap(result.Item, &i)
	if err != nil {
		return nil, err
	}
	return &i.Stack, nil
}

func ddbClusterQuery(sess *session.Session, cluster *string, addon *string) (*[]map[string]*dynamodb.AttributeValue, error) {
	ddbSvc := dynamodb.New(sess)
	result, err := ddbSvc.Query(&dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1 AND begins_with(secondary_id,:id2)"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id1": {S: aws.String("CLUSTERS")},
			":id2": {S: aws.String(fmt.Sprintf("%s#%s#", *cluster, *addon))},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Items == nil {
		return nil, fmt.Errorf("could not find any AppPack %s stacks on %s cluster", strings.ToLower(*addon), *cluster)
	}
	return &result.Items, nil
}

func isHostedZoneForDomain(dnsName string, hostedZone *route53.HostedZone) bool {
	return strings.HasSuffix(dnsName, strings.Trim(*hostedZone.Name, "."))
}

// hostedZoneForDomain searches AWS Hosted Zones for a place for this domain
func hostedZoneForDomain(sess *session.Session, dnsName string) (*route53.HostedZone, error) {
	r53Svc := route53.New(sess)
	nameParts := strings.Split(dnsName, ".")
	// keep stripping off subdomains until a match is found
	for i := range nameParts {
		input := route53.ListHostedZonesByNameInput{
			DNSName: aws.String(strings.Join(nameParts[i:], ".")),
		}
		resp, err := r53Svc.ListHostedZonesByName(&input)
		if err != nil {
			return nil, err
		}
		for _, zone := range resp.HostedZones {
			if isHostedZoneForDomain(dnsName, zone) && *zone.Config.PrivateZone != true {
				return zone, nil
			}
		}
	}
	return nil, fmt.Errorf("no hosted zones found for %s", dnsName)
}

func listStacks(sess *session.Session, cluster *string, addon string) ([]string, error) {
	items, err := ddbClusterQuery(sess, cluster, &addon)
	if err != nil {
		return nil, err
	}
	i := []stackItem{}
	err = dynamodbattribute.UnmarshalListOfMaps(*items, &i)
	if err != nil {
		return nil, err
	}
	stacks := []string{}
	var stack Stack
	for idx := range i {
		stack = i[idx].Stack
		if len(stack.DatabaseEngine) > 0 {
			stacks = append(stacks, fmt.Sprintf("%s (%s)", stack.Name, stack.DatabaseEngine))
		} else {
			stacks = append(stacks, stack.Name)
		}
	}
	return stacks, nil
}

func makeDatabaseQuestion(sess *session.Session, cluster *string) (*survey.Question, error) {
	databases, err := listStacks(sess, cluster, "DATABASE")
	if err != nil {
		return nil, err
	}
	if len(databases) == 0 {
		return nil, fmt.Errorf("no AppPack databases are setup on %s cluster", *cluster)
	}
	defaultDatabase := databases[0]
	return &survey.Question{
		Name: "addon-database-name",
		Prompt: &survey.Select{
			Message: "Select a database cluster",
			Options: databases,
			Default: defaultDatabase,
		},
	}, err
}

func makeRedisQuestion(sess *session.Session, cluster *string) (*survey.Question, error) {
	instances, err := listStacks(sess, cluster, "REDIS")
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, fmt.Errorf("no AppPack Redis instances are setup on %s cluster", *cluster)
	}
	defaultInstance := instances[0]
	return &survey.Question{
		Name: "addon-redis-name",
		Prompt: &survey.Select{
			Message: "Select a Redis instance",
			Options: instances,
			Default: defaultInstance,
		},
	}, err
}

func addQuestionFromFlag(flag *pflag.Flag, questions *[]*survey.Question, override *survey.Question) {
	if !flag.Changed {
		if override != nil {
			*questions = append(*questions, override)
		} else if flag.DefValue == "true" {
			*questions = append(*questions, &survey.Question{
				Name:   flag.Name,
				Prompt: &survey.Select{Message: flag.Usage, Options: []string{"yes", "no"}, FilterMessage: "", Default: "yes"},
			})
		} else if flag.DefValue == "false" {
			*questions = append(*questions, &survey.Question{
				Name:   flag.Name,
				Prompt: &survey.Select{Message: flag.Usage, Options: []string{"yes", "no"}, FilterMessage: "", Default: "no"},
			})
		} else {
			*questions = append(*questions, &survey.Question{
				Name:   flag.Name,
				Prompt: &survey.Input{Message: flag.Usage, Default: flag.DefValue},
			})
		}
	}
}

func getArgValue(cmd *cobra.Command, answers *map[string]interface{}, name string, required bool) *string {
	var val string
	flag := cmd.Flags().Lookup(name)
	// if the flag is set, use that value
	if flag.Changed {
		val = flag.Value.String()
		return &val
	}
	// otherwise, check if there is a matching answer
	answer, ok := (*answers)[name]
	if ok {
		switch v := answer.(type) {
		case *string:
			return answer.(*string)
		case string:
			val, _ = answer.(string)
			return &val
		case survey.OptionAnswer:
			return aws.String(answer.(survey.OptionAnswer).Value)
		default:
			fmt.Printf("Unexpected type, %T\n", v)
			return nil
		}
	}
	// finally, if it is required and a value was not supplied, raise an error
	if required {
		if len(flag.DefValue) > 0 {
			return &flag.DefValue
		}
		checkErr(fmt.Errorf("'--%s' is required", name))
	}
	return &flag.DefValue
}

func askForMissingArgs(cmd *cobra.Command, overrideQuestions *map[string]*survey.Question) (*map[string]interface{}, error) {
	var questions []*survey.Question
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name != "help" {
			var override *survey.Question
			if overrideQuestions != nil {
				override = (*overrideQuestions)[flag.Name]
			} else {
				override = nil
			}
			addQuestionFromFlag(flag, &questions, override)
		}
	})
	answers := make(map[string]interface{})
	if err := survey.Ask(questions, &answers); err != nil {
		return nil, err
	}
	return &answers, nil
}

func stackFromDDBItem(sess *session.Session, secondaryID string) (*cloudformation.Stack, error) {
	ddbSvc := dynamodb.New(sess)
	result, err := ddbSvc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("apppack"),
		Key: map[string]*dynamodb.AttributeValue{
			"primary_id": {
				S: aws.String("CLUSTERS"),
			},
			"secondary_id": {
				S: aws.String(secondaryID),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("could not find CLUSTERS/%s", secondaryID)
	}
	i := stackItem{}
	err = dynamodbattribute.UnmarshalMap(result.Item, &i)
	if err != nil {
		return nil, err
	}
	cfnSvc := cloudformation.New(sess)
	stacks, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &i.Stack.StackID,
	})
	if err != nil {
		return nil, err
	}
	if len(stacks.Stacks) == 0 {
		return nil, fmt.Errorf("no stacks found with ID %s", i.Stack.StackID)
	}
	return stacks.Stacks[0], nil
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func isTruthy(val *string) bool {
	return *val == "yes" || *val == "true"
}

func enabledString(val bool) string {
	if val {
		return "enabled"
	}
	return "disabled"
}

func generatePassword() string {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	length := 30
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "create AppPack resources in your AWS account",
	Long: `Use subcommands to create AppPack resources in your account.
	
These currently require AWS authentication credentials to operate unlike app-specific commands which use AppPack for authentication.
`,
	DisableFlagsInUseLine: true,
}

// accountCmd represents the create command
var accountCmd = &cobra.Command{
	Use:                   "account",
	Short:                 "setup resources for your AppPack account",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		sess, err := awsSession()
		checkErr(err)
		ssmSvc := ssm.New(sess)
		_, err = ssmSvc.GetParameter(&ssm.GetParameterInput{
			Name: aws.String("/apppack/account"),
		})

		if err == nil {
			checkErr(fmt.Errorf("account already exists"))
		}
		if createChangeSet {
			fmt.Println("Creating Cloudformation Change Set for account-level resources...")
		} else {
			fmt.Println("Creating account-level resources...")
		}
		startSpinner()
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("apppack:account"), Value: aws.String("true")},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}

		input := cloudformation.CreateStackInput{
			StackName:   aws.String("apppack-account"),
			TemplateURL: aws.String(accountFormationURL),
			Parameters: []*cloudformation.Parameter{
				{
					ParameterKey:   aws.String("AppPackRoleExternalId"),
					ParameterValue: aws.String(strings.Replace(uuid.New().String(), "-", "", -1)),
				},
			},
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags:         cfnTags,
		}
		var statusURL string
		if createChangeSet {
			changeSet, err := createChangeSetAndWait(sess, &input)
			Spinner.Stop()
			checkErr(err)
			statusURL = fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", *sess.Config.Region, url.QueryEscape(*changeSet.ChangeSetId))
			if *changeSet.Status != "CREATE_COMPLETE" {
				checkErr(fmt.Errorf("Stack ChangeSet creation Failed.\nView status at %s", statusURL))
			} else {
				fmt.Println("View ChangeSet at:")
				fmt.Println(aurora.White(statusURL))
				fmt.Println("Once your stack is created send the 'Outputs' to support@apppack.io for account approval.")
			}
		} else {
			stack, err := createStackAndWait(sess, &input)
			Spinner.Stop()
			checkErr(err)
			statusURL := fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", *sess.Config.Region, url.QueryEscape(*stack.StackId))
			if *stack.StackStatus != "CREATE_COMPLETE" {
				checkErr(fmt.Errorf("Stack creation Failed.\nView status at %s", statusURL))
			} else {
				printSuccess("AppPack account created")
				fmt.Println(aurora.Bold("Send the following information to support@apppack.io for account approval:"))
				for _, output := range stack.Outputs {
					fmt.Println(fmt.Sprintf("%s: %s", *output.OutputKey, *output.OutputValue))
				}

			}
		}

	},
}

// createRedisCmd represents the create redis command
var createRedisCmd = &cobra.Command{
	Use:                   "redis [<name>]",
	Short:                 "setup resources for an AppPack Redis instance",
	Long:                  "*Requires AWS credentials.*\nCreates an AppPack Redis instance. If a `<name>` is not provided, the default name, `apppack` will be used.\nRequires AWS credentials.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var name string
		if len(args) == 0 {
			name = "apppack"
		} else {
			name = args[0]
		}
		sess, err := awsSession()
		checkErr(err)
		answers := make(map[string]interface{})
		if !nonInteractive {
			questions := []*survey.Question{}
			clusterQuestion, err := makeClusterQuestion(sess, aws.String("AppPack Cluster to use for Redis"))
			checkErr(err)
			questions = append(questions, clusterQuestion)
			addQuestionFromFlag(cmd.Flags().Lookup("multi-az"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("instance-class"), &questions, nil)
			if err := survey.Ask(questions, &answers); err != nil {
				checkErr(err)
			}
		}
		cluster := getArgValue(cmd, &answers, "cluster", true)
		// check if a redis already exists on the cluster
		_, err = getDDBClusterItem(sess, cluster, "REDIS", &name)
		if err == nil {
			checkErr(fmt.Errorf(fmt.Sprintf("a Redis instance named %s already exists on the cluster %s", name, *cluster)))
		}
		clusterStack, err := stackFromDDBItem(sess, fmt.Sprintf("CLUSTER#%s", *cluster))
		checkErr(err)
		var multiAZParameter string
		if *(getArgValue(cmd, &answers, "multi-az", false)) == "true" {
			multiAZParameter = "yes"
		} else {
			multiAZParameter = "no"
		}
		if createChangeSet {
			fmt.Println("Creating Cloudformation Change Set for Redis resources...")
		} else {
			fmt.Println("Creating Redis resources, this may take a few minutes...")
		}
		startSpinner()
		authToken := fmt.Sprintf(redisAuthTokenParameterTmpl, name)
		ssmSvc := ssm.New(sess)
		_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
			Name:  &authToken,
			Value: aws.String(generatePassword()),
			Type:  aws.String("SecureString"),
		})
		checkErr(err)
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("apppack:redis"), Value: &name},
			{Key: aws.String("apppack:cluster"), Value: cluster},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}

		input := cloudformation.CreateStackInput{
			StackName:   aws.String(fmt.Sprintf(redisStackNameTmpl, name)),
			TemplateURL: aws.String(redisFormationURL),
			Parameters: []*cloudformation.Parameter{
				{
					ParameterKey:   aws.String("Name"),
					ParameterValue: &name,
				},
				{
					ParameterKey:   aws.String("ClusterStackName"),
					ParameterValue: clusterStack.StackName,
				},
				{
					ParameterKey:   aws.String("AuthTokenParameter"),
					ParameterValue: &authToken,
				},
				{
					ParameterKey:   aws.String("InstanceClass"),
					ParameterValue: getArgValue(cmd, &answers, "instance-class", true),
				},
				{
					ParameterKey:   aws.String("MultiAZ"),
					ParameterValue: &multiAZParameter,
				},
			},
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags:         cfnTags,
		}
		err = createStackOrChangeSet(sess, &input, createChangeSet, fmt.Sprintf("%s Redis instance", name))
		checkErr(err)
	},
}

func verifySourceCredentials(sess *session.Session, repositoryType string, interactive bool) error {
	codebuildSvc := codebuild.New(sess)
	sourceCredentialsOutput, err := codebuildSvc.ListSourceCredentials(&codebuild.ListSourceCredentialsInput{})
	if err != nil {
		return err
	}
	hasCredentials := false
	for _, cred := range sourceCredentialsOutput.SourceCredentialsInfos {
		if *cred.ServerType == repositoryType {
			hasCredentials = true
		}
	}
	if !hasCredentials {
		var friendlySourceName string
		if repositoryType == "BITBUCKET" {
			friendlySourceName = "Bitbucket"
		} else {
			friendlySourceName = "GitHub"
		}
		Spinner.Stop()
		printWarning(fmt.Sprintf("CodeBuild needs to be authenticated to access your repository at %s", friendlySourceName))
		fmt.Println("On the CodeBuild new project page:")
		fmt.Printf("    1. Scroll to the %s section\n", aurora.Bold("Source"))
		fmt.Printf("    2. Select %s for the %s\n", aurora.Bold(friendlySourceName), aurora.Bold("Source provider"))
		fmt.Printf("    3. Keep the default %s\n", aurora.Bold("Connect using OAuth"))
		fmt.Printf("    4. Click %s\n", aurora.Bold(fmt.Sprintf("Connect to %s", friendlySourceName)))
		fmt.Printf("    5. Click %s in the popup window\n\n", aurora.Bold("Confirm"))
		newProjectURL := fmt.Sprintf("https://%s.console.aws.amazon.com/codesuite/codebuild/project/new", *sess.Config.Region)
		if !interactive {
			fmt.Printf("Visit %s to complete the authentication\n", newProjectURL)
			fmt.Println("No further steps are necessary. After you've completed the authentication, re-run this command.")
			os.Exit(1)
		}
		creds, err := sess.Config.Credentials.Get()
		if err != nil {
			return err
		}
		url, err := auth.GetConsoleURL(&creds, newProjectURL)
		if err != nil {
			return err
		}
		if isatty.IsTerminal(os.Stdin.Fd()) {
			fmt.Println("Opening the CodeBuild new project page now...")
			browser.OpenURL(*url)
		} else {
			fmt.Printf("Visit the following URL to authenticate: %s", *url)
		}
		pauseUntilEnter("Finish authentication in your web browser then press ENTER to continue.")
		return verifySourceCredentials(sess, repositoryType, interactive)
	}
	return nil
}

// appCmd represents the create command
var appCmd = &cobra.Command{
	Use:                   "app <name>",
	Short:                 "create an AppPack application",
	Long:                  "*Requires AWS credentials.*",
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		answers := make(map[string]interface{})
		var databaseAddonEnabled bool
		var redisAddonEnabled bool
		name := args[0]
		sess, err := awsSession()
		checkErr(err)
		if !nonInteractive {
			questions := []*survey.Question{}
			clusterQuestion, err := makeClusterQuestion(sess, aws.String("AppPack Cluster to use for app"))
			checkErr(err)
			questions = append(questions, clusterQuestion)
			addQuestionFromFlag(cmd.Flags().Lookup("repository"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("branch"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("domain"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("healthcheck-path"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("addon-private-s3"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("addon-public-s3"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("addon-database"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("addon-redis"), &questions, nil)
			if err := survey.Ask(questions, &answers); err != nil {
				checkErr(err)
			}
			questions = []*survey.Question{}
			databaseAddonEnabled = isTruthy(getArgValue(cmd, &answers, "addon-database", false))
			if databaseAddonEnabled {
				databaseClusterQuestion, err := makeDatabaseQuestion(sess, getArgValue(cmd, &answers, "cluster", false))
				checkErr(err)
				questions = append(questions, databaseClusterQuestion)
			}
			redisAddonEnabled = isTruthy(getArgValue(cmd, &answers, "addon-redis", false))
			if redisAddonEnabled {
				redisInstanceQuestion, err := makeRedisQuestion(sess, getArgValue(cmd, &answers, "cluster", false))
				checkErr(err)
				questions = append(questions, redisInstanceQuestion)
			}
			addQuestionFromFlag(cmd.Flags().Lookup("addon-sqs"), &questions, nil)
			addQuestionFromFlag(cmd.Flags().Lookup("addon-ses"), &questions, nil)
			if err := survey.Ask(questions, &answers); err != nil {
				checkErr(err)
			}
			questions = []*survey.Question{}
			if isTruthy(getArgValue(cmd, &answers, "addon-ses", false)) {
				addQuestionFromFlag(cmd.Flags().Lookup("addon-ses-domain"), &questions, nil)
			}
			addQuestionFromFlag(cmd.Flags().Lookup("users"), &questions, nil)
			if err := survey.Ask(questions, &answers); err != nil {
				checkErr(err)
			}
		}
		startSpinner()
		cluster := getArgValue(cmd, &answers, "cluster", true)
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("apppack:appName"), Value: &name},
			{Key: aws.String("apppack:cluster"), Value: cluster},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}

		clusterStack, err := stackFromDDBItem(sess, fmt.Sprintf("CLUSTER#%s", *cluster))
		checkErr(err)
		sesDomain := ""
		if isTruthy(getArgValue(cmd, &answers, "addon-ses", false)) {
			sesDomain = *getArgValue(cmd, &answers, "addon-ses-domain", false)
		}
		var databaseStackName string
		if isTruthy(getArgValue(cmd, &answers, "addon-database", false)) {
			databaseDisplay := getArgValue(cmd, &answers, "addon-database-name", false)
			// remove ' (engine)' from the database display text
			database := strings.Split(*databaseDisplay, " ")[0]
			databaseStack, err := getDDBClusterItem(sess, cluster, "DATABASE", &database)
			checkErr(err)
			databaseStackName = strings.Split(databaseStack.StackID, "/")[1]
		} else {
			databaseStackName = ""
		}
		var redisStackName string
		if isTruthy(getArgValue(cmd, &answers, "addon-redis", false)) {
			redis := getArgValue(cmd, &answers, "addon-redis-name", false)
			redisStack, err := getDDBClusterItem(sess, cluster, "REDIS", redis)
			checkErr(err)
			redisStackName = strings.Split(redisStack.StackID, "/")[1]
		} else {
			redisStackName = ""
		}
		repositoryURL := getArgValue(cmd, &answers, "repository", true)
		var repositoryType string
		if strings.Contains(*repositoryURL, "github.com") {
			repositoryType = "GITHUB"
		} else if strings.Contains(*repositoryURL, "bitbucket.org") {
			repositoryType = "BITBUCKET"
		} else {
			checkErr(fmt.Errorf("unknown repository source"))
		}
		err = verifySourceCredentials(sess, repositoryType, !nonInteractive)
		checkErr(err)
		rand.Seed(time.Now().UnixNano())
		input := cloudformation.CreateStackInput{
			StackName:   aws.String(appStackName(name)),
			TemplateURL: aws.String(appFormationURL),
			Parameters: []*cloudformation.Parameter{
				{
					ParameterKey:   aws.String("Branch"),
					ParameterValue: getArgValue(cmd, &answers, "branch", true),
				},
				{
					ParameterKey:   aws.String("Domains"),
					ParameterValue: getArgValue(cmd, &answers, "domain", false),
				},
				{
					ParameterKey:   aws.String("HealthCheckPath"),
					ParameterValue: getArgValue(cmd, &answers, "healthcheck-path", true),
				},
				{
					ParameterKey:   aws.String("LoadBalancerRulePriority"),
					ParameterValue: aws.String(fmt.Sprintf("%d", rand.Intn(50000-1)+1)), // TODO: verify empty slot
				},
				{
					ParameterKey:   aws.String("Name"),
					ParameterValue: &name,
				},
				{
					ParameterKey:   aws.String("ClusterStackName"),
					ParameterValue: clusterStack.StackName,
				},
				{
					ParameterKey:   aws.String("AppPackRoleExternalId"),
					ParameterValue: aws.String(strings.Replace(uuid.New().String(), "-", "", -1)),
				},
				{
					ParameterKey:   aws.String("PrivateS3BucketEnabled"),
					ParameterValue: aws.String(enabledString(isTruthy(getArgValue(cmd, &answers, "addon-private-s3", false)))),
				},
				{
					ParameterKey:   aws.String("PublicS3BucketEnabled"),
					ParameterValue: aws.String(enabledString(isTruthy(getArgValue(cmd, &answers, "addon-public-s3", false)))),
				},
				{
					ParameterKey:   aws.String("SesDomain"),
					ParameterValue: &sesDomain,
				},
				{
					ParameterKey:   aws.String("DatabaseStackName"),
					ParameterValue: &databaseStackName,
				},
				{
					ParameterKey:   aws.String("RedisStackName"),
					ParameterValue: &redisStackName,
				},
				{
					ParameterKey:   aws.String("SQSQueueEnabled"),
					ParameterValue: aws.String(enabledString(isTruthy(getArgValue(cmd, &answers, "addon-sqs", false)))),
				},
				{
					ParameterKey:   aws.String("RepositoryType"),
					ParameterValue: &repositoryType,
				},
				{
					ParameterKey:   aws.String("RepositoryUrl"),
					ParameterValue: repositoryURL,
				},
				{
					ParameterKey:   aws.String("Type"),
					ParameterValue: aws.String("app"),
				},
				{
					ParameterKey:   aws.String("AllowedUsers"),
					ParameterValue: aws.String(strings.Trim(*(getArgValue(cmd, &answers, "users", true)), "[]")),
				},
			},
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags:         cfnTags,
		}
		err = createStackOrChangeSet(sess, &input, createChangeSet, fmt.Sprintf("%s app", name))
		checkErr(err)
		fmt.Println(aurora.White(fmt.Sprintf("  %s app created\n  Push to your git repository to trigger a build or run `apppack -a %s build start`", name, name)))
	},
}

func waitForCloudformationStack(cfnSvc *cloudformation.CloudFormation, stackName string) (*cloudformation.Stack, error) {
	startSpinner()
	stackDesc, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}
	stack := stackDesc.Stacks[0]

	if strings.HasSuffix(*stack.StackStatus, "_COMPLETE") || strings.HasSuffix(*stack.StackStatus, "_FAILED") {
		Spinner.Stop()
		return stack, nil
	}
	stackresources, err := cfnSvc.DescribeStackResources(&cloudformation.DescribeStackResourcesInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}

	inProgress := []string{}
	created := []string{}
	deleted := []string{}
	failed := []string{}
	for _, resource := range stackresources.StackResources {
		// CREATE_IN_PROGRESS | CREATE_FAILED | CREATE_COMPLETE | DELETE_IN_PROGRESS | DELETE_FAILED | DELETE_COMPLETE | DELETE_SKIPPED | UPDATE_IN_PROGRESS | UPDATE_FAILED | UPDATE_COMPLETE | IMPORT_FAILED | IMPORT_COMPLETE | IMPORT_IN_PROGRESS | IMPORT_ROLLBACK_IN_PROGRESS | IMPORT_ROLLBACK_FAILED | IMPORT_ROLLBACK_COMPLETE
		if strings.HasSuffix(*resource.ResourceStatus, "_FAILED") {
			failed = append(failed, *resource.ResourceStatus)
		} else if strings.HasSuffix(*resource.ResourceStatus, "_IN_PROGRESS") {
			inProgress = append(inProgress, *resource.ResourceStatus)
		} else if *resource.ResourceStatus == "CREATE_COMPLETE" {
			created = append(created, *resource.ResourceStatus)
		} else if *resource.ResourceStatus == "DELETE_COMPLETE" {
			deleted = append(deleted, *resource.ResourceStatus)
		}
	}
	status := fmt.Sprintf(" resources: %d in progress / %d created / %d deleted", len(inProgress), len(created), len(deleted))
	if len(failed) > 0 {
		status = fmt.Sprintf("%s / %d failed", status, len(failed))
	}
	Spinner.Suffix = status
	time.Sleep(5 * time.Second)
	return waitForCloudformationStack(cfnSvc, stackName)
}

func init() {

	rootCmd.AddCommand(createCmd)
	createCmd.PersistentFlags().BoolVar(&createChangeSet, "check", false, "check stack in Cloudformation before creating")
	createCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for missing flags")
	createCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to create resources in")

	createCmd.AddCommand(accountCmd)

	createCmd.AddCommand(appCmd)
	appCmd.Flags().SortFlags = false
	appCmd.Flags().StringP("cluster", "c", "apppack", "Cluster name")
	appCmd.Flags().StringP("repository", "r", "", "repository URL, e.g. https://github.com/apppackio/apppack-demo-python.git")
	appCmd.Flags().StringP("branch", "b", "", "branch to setup for continuous deployment")
	appCmd.Flags().StringP("domain", "d", "", "custom domain to route to app (optional)")
	appCmd.Flags().String("healthcheck-path", "/", "path which will return a 200 status code for healthchecks")
	appCmd.Flags().Bool("addon-private-s3", false, "setup private S3 bucket add-on")
	appCmd.Flags().Bool("addon-public-s3", false, "setup public S3 bucket add-on")
	appCmd.Flags().Bool("addon-database", false, "setup database add-on")
	appCmd.Flags().String("addon-database-name", "", "database instance to install add-on")
	appCmd.Flags().Bool("addon-redis", false, "setup Redis add-on")
	appCmd.Flags().String("addon-redis-name", "", "Redis instance to install add-on")
	appCmd.Flags().Bool("addon-sqs", false, "setup SQS Queue add-on")
	appCmd.Flags().Bool("addon-ses", false, "setup SES (Email) add-on (requires manual approval of domain at SES)")
	appCmd.Flags().String("addon-ses-domain", "*", "Ddomain approved for sending via SES add-on. Use '*' for all domains.")
	appCmd.Flags().StringSliceP("users", "u", []string{}, "email addresses for users who can manage the app (comma separated)")

	createCmd.AddCommand(createRedisCmd)
	createRedisCmd.Flags().StringP("cluster", "c", "apppack", "cluster name")
	createRedisCmd.Flags().StringP("instance-class", "i", "cache.t3.micro", "instance class -- see https://aws.amazon.com/elasticache/pricing/#On-Demand_Nodes")
	createRedisCmd.Flags().Bool("multi-az", false, "enable multi-AZ -- see https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html")

}
