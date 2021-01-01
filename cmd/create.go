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
	"bufio"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/google/uuid"
	"github.com/lincolnloop/apppack/auth"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	appFormationURL      = "https://s3.amazonaws.com/paaws-cloudformations/latest/app.json"
	clusterFormationURL  = "https://s3.amazonaws.com/paaws-cloudformations/latest/cluster.json"
	accountFormationURL  = "https://s3.amazonaws.com/paaws-cloudformations/latest/account.json"
	databaseFormationURL = "https://s3.amazonaws.com/paaws-cloudformations/latest/database.json"
)

var createChangeSet bool
var nonInteractive bool

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
	fmt.Println(aurora.Faint(fmt.Sprintf("creating %s", *stackOutput.StackId)))
	describeStacksInput := cloudformation.DescribeStacksInput{StackName: stackInput.StackName}
	err = cfnSvc.WaitUntilStackCreateComplete(&describeStacksInput)
	if err != nil {
		return nil, err
	}
	stack, err := cfnSvc.DescribeStacks(&describeStacksInput)
	if err != nil {
		return nil, err
	}
	return stack.Stacks[0], nil
}

type stackItem struct {
	PrimaryID   string `json:"primary_id"`
	SecondaryID string `json:"secondary_id"`
	Stack       Stack  `json:"value"`
}

type Stack struct {
	StackID   string `json:"stack_id"`
	StackName string `json:"stack_name"`
	Name      string `json:"name"`
}

func listClusters(sess *session.Session) ([]string, error) {
	ddbSvc := dynamodb.New(sess)
	result, err := ddbSvc.Query(&dynamodb.QueryInput{
		TableName:              aws.String("paaws"),
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
		return nil, fmt.Errorf("Could not find any AppPack clusters")
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
		TableName: aws.String("paaws"),
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
		TableName:              aws.String("paaws"),
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
	for idx := range i {
		stacks = append(stacks, i[idx].Stack.Name)
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
		TableName: aws.String("paaws"),
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
		return nil, fmt.Errorf("Could not find CLUSTERS/%s", secondaryID)
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

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create AppPack resources in your AWS account",
	Long: `Use these commands to create AppPack resources in your account.
	
These currently require AWS authentication credentials to operate unlike app-specific commands which use AppPack for authentication.
`,
}

// accountCmd represents the create command
var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Setup resources for your AppPack account",
	Long:  `Setup resources for your AppPack account`,
	Run: func(cmd *cobra.Command, args []string) {
		answers, err := askForMissingArgs(cmd, nil)
		checkErr(err)
		sess := session.Must(session.NewSession())
		ssmSvc := ssm.New(sess)
		_, err = ssmSvc.GetParameter(&ssm.GetParameterInput{
			Name: aws.String("/paaws/account"),
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
		tags := []*ssm.Tag{
			{Key: aws.String("paaws:account"), Value: aws.String("true")},
			{Key: aws.String("paaws"), Value: aws.String("true")},
		}
		_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
			Name:  aws.String("/paaws/account/dockerhub-access-token"),
			Value: getArgValue(cmd, answers, "dockerhub-access-token", true),
			Type:  aws.String("SecureString"),
			Tags:  tags,
		})
		checkErr(err)
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("paaws:account"), Value: aws.String("true")},
			{Key: aws.String("paaws"), Value: aws.String("true")},
		}

		input := cloudformation.CreateStackInput{
			StackName:   aws.String("paaws-account"),
			TemplateURL: aws.String(accountFormationURL),
			Parameters: []*cloudformation.Parameter{
				{
					ParameterKey:   aws.String("PaawsRoleExternalId"),
					ParameterValue: aws.String(strings.Replace(uuid.New().String(), "-", "", -1)),
				},
				{
					ParameterKey:   aws.String("DockerhubUsername"),
					ParameterValue: getArgValue(cmd, answers, "dockerhub-username", true),
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
			statusURL = fmt.Sprintf("https://console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", url.QueryEscape(*changeSet.ChangeSetId))
			if *changeSet.Status != "CREATE_COMPLETE" {
				checkErr(fmt.Errorf("Stack ChangeSet creation Failed.\nView status at %s", statusURL))
			} else {
				fmt.Println("View ChangeSet at:")
				fmt.Println(aurora.White(statusURL))
				fmt.Println("Once your stack is created send the 'Outputs' to pete@lincolnloop.com for account approval.")
			}
		} else {
			stack, err := createStackAndWait(sess, &input)
			Spinner.Stop()
			checkErr(err)
			statusURL := fmt.Sprintf("https://console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", url.QueryEscape(*stack.StackId))
			if *stack.StackStatus != "CREATE_COMPLETE" {
				checkErr(fmt.Errorf("Stack creation Failed.\nView status at %s", statusURL))
			} else {
				printSuccess("AppPack account created")
				fmt.Println(aurora.Bold("Send the following information to pete@lincolnloop.com for account approval:"))
				for _, output := range stack.Outputs {
					fmt.Println(fmt.Sprintf("%s: %s", *output.OutputKey, *output.OutputValue))
				}

			}
		}

	},
}

// createClusterCmd represents the create command
var createClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Setup resources for an AppPack cluster",
	Long:  `Setup resources for an AppPack cluster`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		answers, err := askForMissingArgs(cmd, nil)
		var clusterName string
		if len(args) == 0 {
			clusterName = "apppack"
		} else {
			clusterName = args[0]
		}
		checkErr(err)
		sess := session.Must(session.NewSession())
		_, err = stackFromDDBItem(sess, fmt.Sprintf("CLUSTER#%s", clusterName))
		if err == nil {
			checkErr(fmt.Errorf("cluster %s already exists", clusterName))
		}
		if createChangeSet {
			fmt.Println("Creating Cloudformation Change Set for cluster resources...")
		} else {
			fmt.Println("Creating cluster resources...")
		}
		startSpinner()
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("paaws:cluster"), Value: &clusterName},
			{Key: aws.String("paaws"), Value: aws.String("true")},
		}

		input := cloudformation.CreateStackInput{
			StackName:   aws.String(fmt.Sprintf("apppack-cluster-%s", clusterName)),
			TemplateURL: aws.String(clusterFormationURL),
			Parameters: []*cloudformation.Parameter{
				{
					ParameterKey:   aws.String("Name"),
					ParameterValue: &clusterName,
				},
				{
					ParameterKey: aws.String("AvailabilityZones"),
					ParameterValue: aws.String(strings.Join(
						[]string{fmt.Sprintf("%sa", *sess.Config.Region), fmt.Sprintf("%sb", *sess.Config.Region), fmt.Sprintf("%sc", *sess.Config.Region)},
						",",
					)),
				},
				{
					ParameterKey:   aws.String("Domain"),
					ParameterValue: getArgValue(cmd, answers, "domain", true),
				},
				{
					ParameterKey:   aws.String("HostedZone"),
					ParameterValue: getArgValue(cmd, answers, "hosted-zone-id", true),
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
			statusURL = fmt.Sprintf("https://console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", url.QueryEscape(*changeSet.ChangeSetId))
			if *changeSet.Status != "CREATE_COMPLETE" {
				checkErr(fmt.Errorf("Stack ChangeSet creation Failed.\nView status at %s", statusURL))
			} else {
				fmt.Println("View ChangeSet at:")
				fmt.Println(aurora.White(statusURL))
			}
		} else {
			stack, err := createStackAndWait(sess, &input)
			Spinner.Stop()
			checkErr(err)
			statusURL := fmt.Sprintf("https://console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", url.QueryEscape(*stack.StackId))
			if *stack.StackStatus != "CREATE_COMPLETE" {
				checkErr(fmt.Errorf("Stack creation Failed.\nView status at %s", statusURL))
			} else {
				printSuccess(fmt.Sprintf("AppPack cluster %s created", clusterName))
			}
		}

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
		newProjectURL := "https://console.aws.amazon.com/codesuite/codebuild/project/new"
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
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Finish authentication in your web browser then press ENTER to continue.")
		_, _ = reader.ReadString('\n')
		return verifySourceCredentials(sess, repositoryType, interactive)
	}
	return nil
}

// appCmd represents the create command
var appCmd = &cobra.Command{
	Use:   "app",
	Short: "Create an AppPack application",
	Long:  `Create an AppPack application`,
	Run: func(cmd *cobra.Command, args []string) {
		answers := make(map[string]interface{})
		var databaseAddonEnabled bool
		var redisAddonEnabled bool
		sess := session.Must(session.NewSession())
		if !nonInteractive {
			questions := []*survey.Question{}
			addQuestionFromFlag(cmd.Flags().Lookup("name"), &questions, nil)
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
		name := getArgValue(cmd, &answers, "name", true)
		cluster := getArgValue(cmd, &answers, "cluster", true)
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("paaws:appName"), Value: name},
			{Key: aws.String("paaws:cluster"), Value: cluster},
			{Key: aws.String("paaws"), Value: aws.String("true")},
		}

		clusterStack, err := stackFromDDBItem(sess, fmt.Sprintf("CLUSTER#%s", *cluster))
		checkErr(err)
		sesDomain := ""
		if isTruthy(getArgValue(cmd, &answers, "addon-ses", false)) {
			sesDomain = *getArgValue(cmd, &answers, "addon-ses-domain", false)
		}
		var databaseStackName string
		if isTruthy(getArgValue(cmd, &answers, "addon-database", false)) {
			database := getArgValue(cmd, &answers, "addon-database-name", false)
			databaseStack, err := getDDBClusterItem(sess, cluster, "DATABASE", database)
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
			StackName:   aws.String(fmt.Sprintf("apppack-app-%s", *name)),
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
					ParameterValue: name,
				},
				{
					ParameterKey:   aws.String("ClusterStackName"),
					ParameterValue: clusterStack.StackName,
				},
				{
					ParameterKey:   aws.String("PaawsRoleExternalId"),
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
					ParameterValue: getArgValue(cmd, &answers, "users", true),
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
			statusURL = fmt.Sprintf("https://console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", url.QueryEscape(*changeSet.ChangeSetId))
			if *changeSet.Status != "CREATE_COMPLETE" {
				checkErr(fmt.Errorf("Stack ChangeSet creation Failed.\nView status at %s", statusURL))
			} else {
				fmt.Println("View ChangeSet at:")
				fmt.Println(aurora.White(statusURL))
			}
		} else {
			stack, err := createStackAndWait(sess, &input)
			Spinner.Stop()
			checkErr(err)
			statusURL := fmt.Sprintf("https://console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", url.QueryEscape(*stack.StackId))
			if *stack.StackStatus != "CREATE_COMPLETE" {
				checkErr(fmt.Errorf("Stack creation Failed.\nView status at %s", statusURL))
			}
			printSuccess(
				fmt.Sprintf("%s app created.\nPush to your git repository to trigger a build or run `apppack -a %s build start`", *name, *name),
			)
		}

	},
}

func init() {

	rootCmd.AddCommand(createCmd)
	createCmd.PersistentFlags().BoolVar(&createChangeSet, "check", false, "Check stack in Cloudformation before creating")
	createCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for missing flags")

	createCmd.AddCommand(accountCmd)
	appCmd.Flags().SortFlags = false
	accountCmd.Flags().StringP("dockerhub-username", "u", "", "Docker Hub username")
	accountCmd.Flags().StringP("dockerhub-access-token", "t", "", "Docker Hub Access Token (https://hub.docker.com/settings/security)")

	createCmd.AddCommand(appCmd)
	appCmd.Flags().SortFlags = false
	appCmd.Flags().StringP("name", "n", "", "Application name")
	appCmd.Flags().StringP("cluster", "c", "apppack", "Cluster name")
	appCmd.Flags().StringP("repository", "r", "", "Repository URL, e.g. https://github.com/lincolnloop/lincolnloop.git")
	appCmd.Flags().StringP("branch", "b", "", "Branch to setup for continuous deployment")
	appCmd.Flags().StringP("domain", "d", "", "Custom domain to route to app (optional)")
	appCmd.Flags().String("healthcheck-path", "/", "Path which will return a 200 status code for healthchecks")
	appCmd.Flags().Bool("addon-private-s3", false, "Setup private S3 bucket add-on")
	appCmd.Flags().Bool("addon-public-s3", false, "Setup public S3 bucket add-on")
	appCmd.Flags().Bool("addon-database", false, "Setup database add-on")
	appCmd.Flags().String("addon-database-name", "", "Database instance to install add-on")
	appCmd.Flags().Bool("addon-redis", false, "Setup Redis add-on")
	appCmd.Flags().String("addon-redis-name", "", "Redis instance to install add-on")
	appCmd.Flags().Bool("addon-sqs", false, "Setup SQS Queue add-on")
	appCmd.Flags().Bool("addon-ses", false, "Setup SES (Email) add-on (requires manual approval of domain at SES)")
	appCmd.Flags().String("addon-ses-domain", "*", "Domain approved for sending via SES add-on. Use '*' for all domains.")
	appCmd.Flags().StringSliceP("users", "u", []string{}, "Email addresses for users who can manage the app (comma separated)")

	createCmd.AddCommand(createClusterCmd)
	createClusterCmd.Flags().StringP("domain", "d", "", "parent domain for apps in the cluster")
	createClusterCmd.Flags().StringP("hosted-zone-id", "z", "", "AWS Route53 Hosted Zone ID for domain")
}
