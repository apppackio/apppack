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
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apppackio/apppack/ddb"
	"github.com/apppackio/apppack/stacks"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	appFormationURL             = "https://s3.amazonaws.com/apppack-cloudformations/latest/app.json"
	clusterFormationURL         = "https://s3.amazonaws.com/apppack-cloudformations/latest/cluster.json"
	accountFormationURL         = "https://s3.amazonaws.com/apppack-cloudformations/latest/account.json"
	regionFormationURL          = "https://s3.amazonaws.com/apppack-cloudformations/latest/region.json"
	databaseFormationURL        = "https://s3.amazonaws.com/apppack-cloudformations/latest/database.json"
	redisFormationURL           = "https://s3.amazonaws.com/apppack-cloudformations/latest/redis.json"
	customDomainFormationURL    = "https://s3.amazonaws.com/apppack-cloudformations/latest/custom-domain.json"
	accountStackName            = "apppack-account"
	redisStackNameTmpl          = "apppack-redis-%s"
	redisAuthTokenParameterTmpl = "/apppack/redis/%s/auth-token"
	databaseStackNameTmpl       = "apppack-database-%s"
	clusterStackNameTmpl        = "apppack-cluster-%s"
)

var createChangeSet bool
var nonInteractive bool
var region string
var release string

// swap out latest URL for a pre-release
func getReleaseUrl(url string) string {
	if release == "" {
		return url
	}
	return strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", release), 1)
}

func createStackOrChangeSet(sess *session.Session, input *cloudformation.CreateStackInput, changeSet bool, friendlyName string) error {
	cfnSvc := cloudformation.New(sess)
	if changeSet {
		ui.Spinner.Stop()
		fmt.Printf("Creating Cloudformation Change Set for %s...\n", friendlyName)
		ui.StartSpinner()
		changeSet, err := createChangeSetAndWait(cfnSvc, input)
		ui.Spinner.Stop()
		if err != nil {
			return err
		}
		statusURL := cloudformationStackURL(sess.Config.Region, changeSet.ChangeSetId)
		if *changeSet.Status != "CREATE_COMPLETE" {
			return fmt.Errorf("Stack ChangeSet creation Failed.\nView status at %s", statusURL)
		}
		fmt.Println("View ChangeSet at:")
		fmt.Println(aurora.White(statusURL))
	} else {
		ui.Spinner.Stop()
		fmt.Printf("Creating %s resources...\n", friendlyName)
		ui.StartSpinner()
		stack, err := createStackAndWait(cfnSvc, input, true)
		ui.Spinner.Stop()
		if err != nil {
			return err
		}
		if *stack.StackStatus != "CREATE_COMPLETE" {
			return fmt.Errorf("Stack creation failed.\nView status at %s", cloudformationStackURL(sess.Config.Region, stack.StackId))
		}
		ui.PrintSuccess(fmt.Sprintf("created %s", friendlyName))
	}
	return nil
}

func createChangeSetAndWait(cfnSvc *cloudformation.CloudFormation, stackInput *cloudformation.CreateStackInput) (*cloudformation.DescribeChangeSetOutput, error) {
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

func createStackAndWait(cfnSvc *cloudformation.CloudFormation, stackInput *cloudformation.CreateStackInput, retry bool) (*cloudformation.Stack, error) {
	stackOutput, err := cfnSvc.CreateStack(stackInput)
	if err != nil {
		return nil, err
	}
	ui.Spinner.Stop()
	fmt.Println(aurora.Faint(*stackOutput.StackId))
	stack, err := waitForCloudformationStack(cfnSvc, *stackInput.StackName)
	if err != nil {
		return nil, err
	}
	if retry && *stack.StackStatus == "ROLLBACK_COMPLETE" {
		stack, err = retryStackCreation(cfnSvc, stack.StackId, stackInput)
	}
	return stack, err
}

// retryStackCreation will attempt to destroy and recreate the stack
func retryStackCreation(cfnSvc *cloudformation.CloudFormation, stackID *string, input *cloudformation.CreateStackInput) (*cloudformation.Stack, error) {
	ui.PrintWarning("stack creation failed")
	fmt.Println("retrying operation... deleting and recreating stack")
	sentry.CaptureException(fmt.Errorf("Stack creation failed: %s", *input.StackName))
	defer sentry.Flush(time.Second * 5)
	_, err := cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{StackName: stackID})
	if err != nil {
		return nil, err
	}
	stack, err := waitForCloudformationStack(cfnSvc, *stackID)
	checkErr(err)
	if *stack.StackStatus != "DELETE_COMPLETE" {
		err = fmt.Errorf("Stack destruction failed: %s", *stack.StackName)
		sentry.CaptureException(err)
		fmt.Printf("%s", aurora.Bold(aurora.White(
			fmt.Sprintf("Unable to destroy stack. Check Cloudformation console for more details:\n%s", cloudformationStackURL(&region, stackID)),
		)))
		return nil, err
	}
	fmt.Println("successfully deleted failed stack...")
	return createStackAndWait(cfnSvc, input, false)
}

func cloudformationStackURL(region, stackID *string) string {
	return fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", *region, url.QueryEscape(*stackID))
}

// HasSameItems verifies two string slices contain the same elements
func HasSameItems(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// checkHostedZone prompts the user if the NS records for the domain don't match what AWS expects
func checkHostedZone(sess *session.Session, zone *route53.HostedZone) error {
	r53svc := route53.New(sess)
	results, err := net.LookupNS(*zone.Name)
	if err != nil {
		return err
	}
	actualServers := []string{}
	for _, r := range results {
		actualServers = append(actualServers, strings.TrimSuffix(r.Host, "."))
	}
	expectedServers := []string{}
	resp, err := r53svc.GetHostedZone(&route53.GetHostedZoneInput{Id: zone.Id})
	if err != nil {
		return err
	}
	for _, ns := range resp.DelegationSet.NameServers {
		expectedServers = append(expectedServers, strings.TrimSuffix(*ns, "."))
	}
	if HasSameItems(actualServers, expectedServers) {
		return nil
	}
	ui.Spinner.Stop()
	ui.PrintWarning(fmt.Sprintf("%s doesn't appear to be using AWS' domain servers", strings.TrimSuffix(*zone.Name, ".")))
	fmt.Printf("Expected:\n  %s\n\n", strings.Join(expectedServers, "\n  "))
	fmt.Printf("Actual:\n  %s\n\n", strings.Join(actualServers, "\n  "))
	fmt.Printf("If nameservers are not setup properly, TLS certificate creation will fail.\n")
	ui.PauseUntilEnter("Once you've verified the nameservers are correct, press ENTER to continue.")
	return nil
}

func makeDatabaseQuestion(sess *session.Session, cluster *string) (*survey.Question, error) {
	databases, err := ddb.ListStacks(sess, cluster, "DATABASE")
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
	instances, err := ddb.ListStacks(sess, cluster, "REDIS")
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
	
These require administrator access.
`,
	DisableFlagsInUseLine: true,
}

// createRedisCmd represents the create redis command
var createRedisCmd = &cobra.Command{
	Use:                   "redis [<name>]",
	Short:                 "setup resources for an AppPack Redis instance",
	Long:                  "*Requires admin permissions.*\nCreates an AppPack Redis instance. If a `<name>` is not provided, the default name, `apppack` will be used.\nRequires admin permissions.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		var name string
		if len(args) == 0 {
			name = "apppack"
		} else {
			name = args[0]
		}
		CreateStackCommand(sess, &StackCommandOpts{
			StackName: name,
			StackType: "Redis",
			Flags:     cmd.Flags(),
			Stack: &stacks.RedisStack{
				Parameters: &stacks.RedisStackParameters{},
			},
		})
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
		_, err = ddb.GetClusterItem(sess, cluster, "REDIS", &name)
		if err == nil {
			checkErr(fmt.Errorf(fmt.Sprintf("a Redis instance named %s already exists on the cluster %s", name, *cluster)))
		}
		clusterStack, err := ddb.StackFromItem(sess, fmt.Sprintf("CLUSTER#%s", *cluster))
		checkErr(err)
		var multiAZParameter string
		if isTruthy((getArgValue(cmd, &answers, "multi-az", false))) {
			multiAZParameter = "yes"
		} else {
			multiAZParameter = "no"
		}
		if createChangeSet {
			fmt.Println("Creating Cloudformation Change Set for Redis resources...")
		} else {
			fmt.Println("Creating Redis resources, this may take a few minutes...")
		}
		ui.StartSpinner()

		input := cloudformation.CreateStackInput{
			StackName:   aws.String(fmt.Sprintf(redisStackNameTmpl, name)),
			TemplateURL: aws.String(getReleaseUrl(redisFormationURL)),
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
					ParameterKey:   aws.String("InstanceClass"),
					ParameterValue: getArgValue(cmd, &answers, "instance-class", true),
				},
				{
					ParameterKey:   aws.String("MultiAZ"),
					ParameterValue: &multiAZParameter,
				},
			},
		}
	},
}

func CreateStackCommand(sess *session.Session, opts *StackCommandOpts) {
	checkErr(stacks.LoadStack(opts.Stack, opts.Flags))
	ui.Spinner.Stop()
	fmt.Print(aurora.Green(fmt.Sprintf("🏗  Creating %s `%s` in %s", opts.StackType, opts.StackName, *sess.Config.Region)).String())
	if CurrentAccountRole != nil {
		fmt.Print(aurora.Green(fmt.Sprintf(" on %s", CurrentAccountRole.GetAccountName())).String())
	}
	fmt.Println()
	sess, err := adminSession(MaxSessionDurationSeconds)
	checkErr(err)
	if !nonInteractive {
		checkErr(opts.Stack.AskQuestions(sess))
	}
	ui.StartSpinner()
	if createChangeSet {
		checkErr(stacks.CreateStackChangeset(opts.Stack, sess, &opts.StackName, &release))
	} else {
		checkErr(stacks.CreateStack(opts.Stack, sess, &opts.StackName, &release))
	}
	ui.Spinner.Stop()
	ui.PrintSuccess(fmt.Sprintf("updated %s stack for %s", opts.StackType, opts.StackName))
}

// appCmd represents the create app command
var appCmd = &cobra.Command{
	Use:                   "app <name>",
	Short:                 "create an AppPack application",
	Long:                  "*Requires admin permissions.*",
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		name := args[0]
		CreateStackCommand(sess, &StackCommandOpts{
			StackName: name,
			StackType: "app",
			Flags:     cmd.Flags(),
			Stack: &stacks.AppStack{
				Parameters: &stacks.AppStackParameters{},
				Pipeline:   false,
			},
		})
		fmt.Println(aurora.White(fmt.Sprintf("Push to your git repository to trigger a build or run `apppack -a %s build start`", name)))
	},
}

// pipelineCmd represents the create pipeline command
var pipelineCmd = &cobra.Command{
	Use:                   "pipeline <name>",
	Short:                 "create an AppPack pipeline",
	Long:                  "*Requires admin permissions.*",
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		name := args[0]
		CreateStackCommand(sess, &StackCommandOpts{
			StackName: name,
			StackType: "pipeline",
			Flags:     cmd.Flags(),
			Stack: &stacks.AppStack{
				Parameters: &stacks.AppStackParameters{},
				Pipeline:   true,
			},
		})
	},
}

// stackHasFailure is used to track the first occurrence of a failure while waiting for a Cloudformation stack
var stackHasFailure = false

// waitForCloudformationStack displays the progress of a Stack while it waits for it to complete
func waitForCloudformationStack(cfnSvc *cloudformation.CloudFormation, stackName string) (*cloudformation.Stack, error) {
	ui.StartSpinner()
	stackDesc, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}
	stack := stackDesc.Stacks[0]

	if strings.HasSuffix(*stack.StackStatus, "_COMPLETE") || strings.HasSuffix(*stack.StackStatus, "_FAILED") {
		ui.Spinner.Stop()
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
			// only warn on the first failure
			// failures will cascade and end up being extra noise
			if !stackHasFailure {
				ui.Spinner.Stop()
				ui.PrintError(fmt.Sprintf("%s failed: %s", *resource.LogicalResourceId, *resource.ResourceStatusReason))
				ui.StartSpinner()
				stackHasFailure = true
			}
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
	ui.Spinner.Suffix = status
	time.Sleep(5 * time.Second)
	return waitForCloudformationStack(cfnSvc, stackName)
}

func init() {

	rootCmd.AddCommand(createCmd)
	createCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", "AWS account ID or alias (not needed if you are only the administrator of one account)")
	createCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	createCmd.PersistentFlags().BoolVar(&createChangeSet, "check", false, "check stack in Cloudformation before creating")
	createCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for missing flags")
	createCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to create resources in")
	createCmd.PersistentFlags().StringVar(&release, "release", "", "Specify a specific pre-release stack")
	createCmd.PersistentFlags().MarkHidden("release")

	createCmd.AddCommand(appCmd)
	appCmd.Flags().SortFlags = false
	appCmd.Flags().String("cluster", "apppack", "Cluster name")
	appCmd.Flags().Bool("ec2", false, "run on EC2 instances (requires EC2 enabled cluster)")
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
	appCmd.Flags().String("addon-ses-domain", "*", "domain approved for sending via SES add-on. Use '*' for all domains.")
	appCmd.Flags().StringSliceP("users", "u", []string{}, "email addresses for users who can manage the app (comma separated)")
	appCmd.Flags().Bool("disable-build-webhook", false, "disable creation of a webhook on the repo to automatically trigger builds on push")

	createCmd.AddCommand(pipelineCmd)
	pipelineCmd.Flags().SortFlags = false
	pipelineCmd.Flags().String("cluster", "apppack", "Cluster name")
	pipelineCmd.Flags().Bool("ec2", false, "run on EC2 instances (requires EC2 enabled cluster)")
	pipelineCmd.Flags().StringP("repository", "r", "", "repository URL, e.g. https://github.com/apppackio/apppack-demo-python.git")
	pipelineCmd.Flags().String("healthcheck-path", "/", "path which will return a 200 status code for healthchecks")
	pipelineCmd.Flags().Bool("addon-private-s3", false, "setup private S3 bucket add-on")
	pipelineCmd.Flags().Bool("addon-public-s3", false, "setup public S3 bucket add-on")
	pipelineCmd.Flags().Bool("addon-database", false, "setup database add-on")
	pipelineCmd.Flags().String("addon-database-name", "", "database instance to install add-on")
	pipelineCmd.Flags().Bool("addon-redis", false, "setup Redis add-on")
	pipelineCmd.Flags().String("addon-redis-name", "", "Redis instance to install add-on")
	pipelineCmd.Flags().Bool("addon-sqs", false, "setup SQS Queue add-on")
	pipelineCmd.Flags().Bool("addon-ses", false, "setup SES (Email) add-on (requires manual approval of domain at SES)")
	pipelineCmd.Flags().String("addon-ses-domain", "*", "domain approved for sending via SES add-on. Use '*' for all domains.")
	pipelineCmd.Flags().StringSliceP("users", "u", []string{}, "email addresses for users who can manage the app (comma separated)")

	createCmd.AddCommand(createRedisCmd)
	createRedisCmd.Flags().String("cluster", "apppack", "cluster name")
	createRedisCmd.Flags().String("instance-class", "cache.t3.micro", "instance class -- see https://aws.amazon.com/elasticache/pricing/#On-Demand_Nodes")
	createRedisCmd.Flags().Bool("multi-az", false, "enable multi-AZ -- see https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html")

}
