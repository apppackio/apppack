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
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/core"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ddb"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var DisableBuildWebhook bool = false

type AppStackParameters struct {
	Type                     string
	Name                     string
	ClusterStackName         string
	RepositoryUrl            string `flag:"repository"`
	Branch                   string `flag:"branch"`
	Domain                   string `flag:"domain"`
	HealthCheckPath          string `flag:"healthcheck-path"`
	HealthcheckInterval      int
	DeregistrationDelay      int
	LoadBalancerRulePriority int
	AppPackRoleExternalId    string
	PrivateS3BucketEnabled   bool   `flag:"addon-private-s3"`
	PublicS3BucketEnabled    bool   `flag:"addon-public-s3"`
	SesDomain                string `flag:"addon-ses-domain"`
	DatabaseStackName        string
	RedisStackName           string
	SQSQueueEnabled          bool `flag:"addon-sqs"`
	RepositoryType           string
	Fargate                  bool
	AllowedUsers             []string
	BuildWebhook             bool
	CustomTaskPolicyArn      string
}

func (p *AppStackParameters) ClusterName() string {
	return strings.TrimPrefix(p.ClusterStackName, "apppack-cluster-")
}

func (p *AppStackParameters) GenerateLoadBalancerPriority() {
	p.LoadBalancerRulePriority = rand.Intn(50000-200) + 200
}

func (p *AppStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	return bridge.StructToCloudformationParameters(p)
}

func (p *AppStackParameters) UpdateFromCloudformation(parameters []*cloudformation.Parameter) error {
	return bridge.CloudformationParametersToStruct(p, parameters)
}

func (p *AppStackParameters) UpdateFromFlags(flags *pflag.FlagSet) error {
	err := ui.FlagsToStruct(p, flags)
	if err != nil {
		return err
	}
	sort.Strings(p.AllowedUsers)
	return nil
}

func (p *AppStackParameters) SetRepositoryType() error {
	if strings.Contains(p.RepositoryUrl, "github.com") {
		p.RepositoryType = "GITHUB"
		return nil
	}
	if strings.Contains(p.RepositoryUrl, "bitbucket.org") {
		p.RepositoryType = "BITBUCKET"
		return nil
	}
	return fmt.Errorf("unknown repository source")
}

func (p *AppStackParameters) InitialQuestions(sess *session.Session) ([]*ui.QuestionExtra, error) {
	questions := []*ui.QuestionExtra{}
	if p.ClusterStackName == "" {
		clusterQuestion, err := makeClusterQuestion(sess, aws.String("cluster to install app into"))
		if err != nil {
			return nil, err
		}

		// TODO
		questions = append(questions, &ui.QuestionExtra{
			Question: clusterQuestion,
			Verbose:  "Which cluster should the app be installed into?",
		})

	}
	sort.Strings(p.AllowedUsers)
	questions = append(questions, []*ui.QuestionExtra{
		{
			Verbose:  "What code repository should this app build from?",
			HelpText: "Use the HTTP URL (e.g., https://github.com/{org}/{repo}.git). BitBucket and Github repositories are supported.",
			Question: &survey.Question{
				Name:     "RepositoryUrl",
				Prompt:   &survey.Input{Message: "Repository URL", Default: p.RepositoryUrl},
				Validate: survey.Required,
			},
		},
		{
			Verbose:  "What branch should this app build from?",
			HelpText: "The deployment pipeline will be triggered on new pushes to this branch.",
			Question: &survey.Question{
				Name:     "Branch",
				Prompt:   &survey.Input{Message: "Branch", Default: p.Branch},
				Validate: survey.Required,
			},
		},
		{
			Verbose:  "Should the app be served on a custom domain? (Optional)",
			HelpText: "By default, the app will automatically be assigned a domain within the cluster. If you'd like it to respond to another domain, enter it here. See https://docs.apppack.io/how-to/custom-domains/ for more info.",
			Question: &survey.Question{
				Name:   "Domain",
				Prompt: &survey.Input{Message: "Custom Domain", Default: p.Domain},
			},
		},
		{
			Verbose:  "What path should be used for healthchecks?",
			HelpText: "Enter a path (e.g., `/-/alive/`) that will always serve a 200 status code when the application is healthy.",
			Question: &survey.Question{
				Name:     "HealthCheckPath",
				Prompt:   &survey.Input{Message: "Healthcheck Path", Default: p.HealthCheckPath},
				Validate: survey.Required,
			},
		},
		{
			Verbose:  "Should a private S3 Bucket be created for this app?",
			HelpText: "The S3 Bucket can be used to store files that should not be publicly accessible. Answering yes will create the bucket and provide its name to the app as a config variable. See https://docs.apppack.io/how-to/using-s3/ for more info.",
			WriteTo:  &ui.BooleanOptionProxy{Value: &p.PrivateS3BucketEnabled},
			Question: &survey.Question{
				Prompt: &survey.Select{Message: "Private S3 Bucket", Options: []string{"yes", "no"}, FilterMessage: "", Default: ui.BooleanAsYesNo(p.PrivateS3BucketEnabled)},
			},
		},
		{
			Verbose:  "Should a public S3 Bucket be created for this app?",
			HelpText: "The S3 Bucket can be used to store files that should be publicly accessible. Answering yes will create the bucket and provide its name to the app as a config variable. See https://docs.apppack.io/how-to/using-s3/ for more info.",
			WriteTo:  &ui.BooleanOptionProxy{Value: &p.PublicS3BucketEnabled},
			Question: &survey.Question{
				Prompt: &survey.Select{Message: "Public S3 Bucket", Options: []string{"yes", "no"}, FilterMessage: "", Default: ui.BooleanAsYesNo(p.PublicS3BucketEnabled)},
			},
		},
		{
			Verbose:  "Should an SQS Queue be created for this app?",
			HelpText: "The SQS Queue can be used to queue up messages between processes. Answering yes will create the queue and provide its name to the app as a config variable. See https://docs.apppack.io/how-to/using-sqs/ for more info.",
			WriteTo:  &ui.BooleanOptionProxy{Value: &p.SQSQueueEnabled},
			Question: &survey.Question{
				Prompt: &survey.Select{Message: "SQS Queue", Options: []string{"yes", "no"}, FilterMessage: "", Default: ui.BooleanAsYesNo(p.SQSQueueEnabled)},
			},
		},
	}...)
	return questions, nil
}
func (p *AppStackParameters) AskInitialQuestions(sess *session.Session) error {
	questions, err := p.InitialQuestions(sess)
	if err != nil {
		return err
	}
	return ui.AskQuestions(questions, p)
}

func (p *AppStackParameters) AskForDatabase(sess *session.Session) (bool, error) {
	enable := p.DatabaseStackName != ""
	err := ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  "Should a database be created for this app?",
			HelpText: "Create a database for the app on a database instance in the cluster. Answering yes will create a user and database and provide the credentials to the app as a config variable. See https://docs.apppack.io/how-to/using-databases/ for more info.",
			WriteTo:  &ui.BooleanOptionProxy{Value: &enable},
			Question: &survey.Question{
				Prompt: &survey.Select{Message: "Database", Options: []string{"yes", "no"}, FilterMessage: "", Default: ui.BooleanAsYesNo(enable)},
			},
		},
	}, p)
	if err != nil {
		return false, err
	}
	return enable, nil
}

// DatabaseStackParameters converts `{name} ({Engine})` -> `{stackName}`
func DatabaseSelectTransform(ans interface{}) interface{} {
	o, ok := ans.(core.OptionAnswer)
	if !ok {
		return ans
	}
	if o.Value != "" {
		parts := strings.Split(o.Value, " ")
		o.Value = fmt.Sprintf(databaseStackNameTmpl, parts[0])
	}
	return o
}

// AskForDatabaseStack gives the user a choice of available database stacks
func (p *AppStackParameters) AskForDatabaseStack(sess *session.Session) error {
	clusterName := p.ClusterName()
	// databases is a list of `{name} ({engine})` for the databases in the cluster
	databases, err := ddb.ListStacks(sess, &clusterName, "DATABASE")
	if err != nil {
		return err
	}
	if len(databases) == 0 {
		return fmt.Errorf("no AppPack databases are setup on %s cluster", clusterName)
	}
	// set the current database as default
	defaultDatabaseIdx := 0
	if p.DatabaseStackName != "" {
		for i, db := range databases {
			name := strings.Split(db, " ")[0]
			if fmt.Sprintf(databaseStackNameTmpl, name) == p.DatabaseStackName {
				defaultDatabaseIdx = i
				break
			}
		}
	}
	err = ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose: "Which database cluster should this app's database be setup on?",
			Question: &survey.Question{
				Name: "DatabaseStackName",
				Prompt: &survey.Select{
					Message: "Database Cluster",
					Options: databases,
					Default: databases[defaultDatabaseIdx],
				},
				Transform: DatabaseSelectTransform,
			},
		},
	}, p)
	if err != nil {
		return err
	}
	return nil
}

// RedisStackParameters converts `{name}` -> `{stackName}`
func RedisSelectTransform(ans interface{}) interface{} {
	o, ok := ans.(core.OptionAnswer)
	if !ok {
		return ans
	}
	if o.Value != "" {
		o.Value = fmt.Sprintf(redisStackNameTmpl, o.Value)
	}
	return o
}

func (p *AppStackParameters) AskForRedis(sess *session.Session) (bool, error) {
	enable := p.RedisStackName != ""
	err := ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  "Should a Redis user be created for this app?",
			HelpText: "Create a Redis user for the app on a Redis instance in the cluster. Answering yes will create a user and provide the credentials to the app as a config variable. See https://docs.apppack.io/how-to/using-redis/ for more info.",
			WriteTo:  &ui.BooleanOptionProxy{Value: &enable},
			Question: &survey.Question{
				Prompt: &survey.Select{
					Message:       "Redis",
					Options:       []string{"yes", "no"},
					FilterMessage: "",
					Default:       ui.BooleanAsYesNo(enable),
				},
			},
		},
	}, p)
	if err != nil {
		return false, err
	}
	return enable, nil
}

// AskForRedisStack gives the user a choice of available Redis stacks
func (p *AppStackParameters) AskForRedisStack(sess *session.Session) error {
	clusterName := p.ClusterName()
	redises, err := ddb.ListStacks(sess, &clusterName, "REDIS")
	if err != nil {
		return err
	}
	if len(redises) == 0 {
		return fmt.Errorf("no AppPack Redis instances are setup on %s cluster", clusterName)
	}
	// set the current database as default
	defaultRedisIdx := 0
	if p.RedisStackName != "" {
		for i, r := range redises {
			if fmt.Sprintf(databaseStackNameTmpl, r) == p.RedisStackName {
				defaultRedisIdx = i
				break
			}
		}
	}
	err = ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose: "Which database cluster should this app's database be setup on?",
			Question: &survey.Question{
				Name: "RedisStackName",
				Prompt: &survey.Select{
					Message: "Redis Cluster",
					Options: redises,
					Default: redises[defaultRedisIdx],
				},
				Transform: RedisSelectTransform,
			},
		},
	}, p)
	if err != nil {
		return err
	}
	return nil
}

func (p *AppStackParameters) AskForSES() (bool, error) {
	enable := p.SesDomain != ""
	err := ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  "Should this app be allowed to send outgoing email via SES?",
			HelpText: "Allow this app to send email via SES. See https://docs.apppack.io/how-to/sending%20email/ for more info.",
			WriteTo:  &ui.BooleanOptionProxy{Value: &enable},
			Question: &survey.Question{
				Prompt: &survey.Select{
					Message:       "SES (email)",
					Options:       []string{"yes", "no"},
					FilterMessage: "",
					Default:       ui.BooleanAsYesNo(enable),
				},
			},
		},
	}, p)
	if err != nil {
		return false, err
	}
	return enable, nil
}

// AskForRedisStack gives the user a choice of available Redis stacks
func (p *AppStackParameters) AskForSESDomain() error {
	err := ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  "What domain should this app be allowed to send from?",
			HelpText: "Only allow outbound email via SES from a specific domain (e.g., example.com). Use `*` to allow sending on any domain approved for sending in SES.",
			Question: &survey.Question{
				Name:     "SesDomain",
				Prompt:   &survey.Input{Message: "SES Approved Domain", Default: p.SesDomain},
				Validate: survey.Required,
			},
		},
	}, p)
	if err != nil {
		return err
	}
	return nil
}

func (p *AppStackParameters) FinalQuestions() ([]*ui.QuestionExtra, error) {
	questions := []*ui.QuestionExtra{}
	// if users already exist, the `access` group of commands should be used
	if len(p.AllowedUsers) == 0 {
		questions = append(questions, &ui.QuestionExtra{
			Verbose:  "Who can manage this app?",
			HelpText: "A list of email addresses (one per line) who have access to manage this app via AppPack.",
			Question: &survey.Question{
				Name:     "AllowedUsers",
				Prompt:   &survey.Multiline{Message: "Users"},
				Validate: survey.Required,
			},
		})
	}
	return questions, nil
}

func (p *AppStackParameters) AskFinalQuestions(sess *session.Session) error {
	questions, err := p.FinalQuestions()
	if err != nil {
		return err
	}
	return ui.AskQuestions(questions, p)
}

// UpdateCustomFields updates fields that aren't one-to-one mappings to flags
func (p *AppStackParameters) UpdateCustomFields(flags *pflag.FlagSet) error {
	// update values from flags if they are set
	flags.Visit(func(f *pflag.Flag) {
		if f.Name == "disable-build-webhook" {
			p.BuildWebhook = !DisableBuildWebhook
		}
		if f.Name == "addon-redis-name" {
			p.RedisStackName = fmt.Sprintf(redisStackNameTmpl, flags.Lookup("addon-redis-name").Value.String())
		}
		if f.Name == "addon-database-name" {
			p.RedisStackName = fmt.Sprintf(databaseStackNameTmpl, flags.Lookup("addon-database-name").Value.String())
		}
	})
	if p.LoadBalancerRulePriority == 0 {
		p.GenerateLoadBalancerPriority()
	}
	if err := p.SetRepositoryType(); err != nil {
		return err
	}
	return nil
}

func NewAppStackParametersFromStack(stack *cloudformation.Stack) (*AppStackParameters, error) {
	p := AppStackParameters{}
	err := p.UpdateFromCloudformation(stack.Parameters)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// modifyAppCmd represents the modify app command
var modifyAppCmd = &cobra.Command{
	Use:     "app",
	Short:   "modify the settings for an app",
	Args:    cobra.ExactArgs(1),
	Example: "apppack modify app <appname>",
	Long: `Modify the settings for an app after creation.

Requires administrator privileges.`,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess, err := adminSession()
		checkErr(err)
		AppName = args[0]
		stack, err := appOrPipelineStack(sess, AppName)
		checkErr(err)
		params, err := NewAppStackParametersFromStack(stack)
		checkErr(err)
		checkErr(params.UpdateFromFlags(cmd.Flags()))
		Spinner.Stop()
		fmt.Println(aurora.Green(fmt.Sprintf("✏️  Updating app `%s` in %s on account %s", AppName, *sess.Config.Region, CurrentAccountRole.GetAccountName())))
		checkErr(params.AskInitialQuestions(sess))
		checkErr(params.UpdateCustomFields(cmd.Flags()))
		wantDB, err := params.AskForDatabase(sess)
		checkErr(err)
		if wantDB {
			checkErr(params.AskForDatabaseStack(sess))
		}
		wantRedis, err := params.AskForRedis(sess)
		checkErr(err)
		if wantRedis {
			checkErr(params.AskForRedisStack(sess))
		}
		wantSES, err := params.AskForSES()
		checkErr(err)
		if wantSES {
			checkErr(params.AskForSESDomain())
		}
		checkErr(params.AskFinalQuestions(sess))
		startSpinner()
		cfnParams, err := params.ToCloudFormationParameters()
		checkErr(err)
		fmt.Println(cfnParams)
		// _, err = updateStackAndWait(sess, &cloudformation.UpdateStackInput{
		// 	StackName:           stack.StackName,
		// 	Parameters:          stack.Parameters,
		// 	UsePreviousTemplate: aws.Bool(true),
		// 	Capabilities:        []*string{aws.String("CAPABILITY_IAM")},
		// })
		checkErr(err)
		Spinner.Stop()
		printSuccess(fmt.Sprintf("updated app stack for %s", AppName))
	},
}

func init() {
	modifyAppCmd.Flags().StringP("repository", "r", "", "repository URL, e.g. https://github.com/apppackio/apppack-demo-python.git")
	modifyAppCmd.Flags().StringP("branch", "b", "", "branch to setup for continuous deployment")
	modifyAppCmd.Flags().StringP("domain", "d", "", "custom domain to route to app (optional)")
	modifyAppCmd.Flags().String("healthcheck-path", "/", "path which will return a 200 status code for healthchecks")
	modifyAppCmd.Flags().Bool("addon-private-s3", false, "setup private S3 bucket add-on")
	modifyAppCmd.Flags().Bool("addon-public-s3", false, "setup public S3 bucket add-on")
	modifyAppCmd.Flags().String("addon-database-name", "", "database instance to install add-on")
	modifyAppCmd.Flags().String("addon-redis-name", "", "Redis instance to install add-on")
	modifyAppCmd.Flags().Bool("addon-sqs", false, "setup SQS Queue add-on")
	modifyAppCmd.Flags().String("addon-ses-domain", "", "domain approved for sending via SES add-on. Use '*' for all domains.")
	modifyAppCmd.Flags().StringSliceP("users", "u", []string{}, "email addresses for users who can manage the app (comma separated)")
	modifyAppCmd.Flags().BoolVar(&DisableBuildWebhook, "disable-build-webhook", false, "disable creation of a webhook on the repo to automatically trigger builds on push")

	// createCmd.AddCommand(pipelineCmd)
	// pipelineCmd.Flags().SortFlags = false
	// pipelineCmd.Flags().StringP("repository", "r", "", "repository URL, e.g. https://github.com/apppackio/apppack-demo-python.git")
	// pipelineCmd.Flags().String("healthcheck-path", "/", "path which will return a 200 status code for healthchecks")
	// pipelineCmd.Flags().Bool("addon-private-s3", false, "setup private S3 bucket add-on")
	// pipelineCmd.Flags().Bool("addon-public-s3", false, "setup public S3 bucket add-on")
	// pipelineCmd.Flags().Bool("addon-database", false, "setup database add-on")
	// pipelineCmd.Flags().String("addon-database-name", "", "database instance to install add-on")
	// pipelineCmd.Flags().Bool("addon-redis", false, "setup Redis add-on")
	// pipelineCmd.Flags().String("addon-redis-name", "", "Redis instance to install add-on")
	// pipelineCmd.Flags().Bool("addon-sqs", false, "setup SQS Queue add-on")
	// pipelineCmd.Flags().Bool("addon-ses", false, "setup SES (Email) add-on (requires manual approval of domain at SES)")
	// pipelineCmd.Flags().String("addon-ses-domain", "*", "domain approved for sending via SES add-on. Use '*' for all domains.")
	// pipelineCmd.Flags().StringSliceP("users", "u", []string{}, "email addresses for users who can manage the app (comma separated)")

}
