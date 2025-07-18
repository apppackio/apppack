package stacks

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/apppackio/apppack/auth"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ddb"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
	"github.com/pkg/browser"
	"github.com/spf13/pflag"
)

type AppStackParameters struct {
	Type                               string
	Name                               string
	ClusterStackName                   string   `flag:"cluster;fmtString:apppack-cluster-%s"`
	RepositoryUrl                      string   `flag:"repository"`
	Branch                             string   `flag:"branch"`
	Domains                            []string `flag:"domains"`
	DefaultAutoscalingAverageCpuTarget int
	HealthCheckPath                    string `flag:"healthcheck-path"`
	HealthcheckInterval                int
	DeregistrationDelay                int
	LoadBalancerRulePriority           int
	LogRetentionDays                   int
	AppPackRoleExternalId              string
	PrivateS3BucketEnabled             bool   `flag:"addon-private-s3"`
	PublicS3BucketEnabled              bool   `flag:"addon-public-s3"`
	SesDomain                          string `flag:"addon-ses-domain"`
	DatabaseStackName                  string `flag:"addon-database-name;fmtString:apppack-database-%s"`
	RedisStackName                     string `flag:"addon-redis-name;fmtString:apppack-redis-%s"`
	SQSQueueEnabled                    bool   `flag:"addon-sqs"`
	RepositoryType                     string
	Fargate                            bool     `flag:"ec2;negate"`
	AllowedUsers                       []string `flag:"users"`
	BuildWebhook                       bool     `flag:"disable-build-webhook;negate"`
	CustomTaskPolicyArn                string
}

var DefaultAppStackParameters = AppStackParameters{
	Type:                               "app",
	HealthCheckPath:                    "/",
	HealthcheckInterval:                30,
	LogRetentionDays:                   30,
	DefaultAutoscalingAverageCpuTarget: 50,
	DeregistrationDelay:                15,
	Fargate:                            true,
	BuildWebhook:                       true,
}

var DefaultPipelineStackParameters = AppStackParameters{
	Type:                               "pipeline",
	HealthCheckPath:                    DefaultAppStackParameters.HealthCheckPath,
	HealthcheckInterval:                DefaultAppStackParameters.HealthcheckInterval,
	LogRetentionDays:                   DefaultAppStackParameters.LogRetentionDays,
	DefaultAutoscalingAverageCpuTarget: DefaultAppStackParameters.DefaultAutoscalingAverageCpuTarget,
	DeregistrationDelay:                DefaultAppStackParameters.DeregistrationDelay,
	Fargate:                            DefaultAppStackParameters.Fargate,
	BuildWebhook:                       DefaultAppStackParameters.BuildWebhook,
}

func (p *AppStackParameters) Import(parameters []*cloudformation.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *AppStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *AppStackParameters) SetInternalFields(_ *session.Session, name *string) error {
	// update values from flags if they are set
	if p.LoadBalancerRulePriority == 0 {
		rand.Seed(time.Now().UnixNano())                        // skipcq: GO-S1033
		p.LoadBalancerRulePriority = rand.Intn(50000-200) + 200 // skipcq: GSC-G404
	}
	if err := p.SetRepositoryType(); err != nil {
		return err
	}
	if p.AppPackRoleExternalId == "" {
		// TODO: This should come from us instead of the user
		p.AppPackRoleExternalId = strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	if p.Name == "" {
		p.Name = *name
	}

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

type AppStack struct {
	Stack      *cloudformation.Stack
	Parameters *AppStackParameters
	Pipeline   bool
}

func (a *AppStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *AppStack) GetStack() *cloudformation.Stack {
	return a.Stack
}

func (a *AppStack) SetStack(stack *cloudformation.Stack) {
	a.Stack = stack
}

func (*AppStack) PostCreate(_ *session.Session) error {
	return nil
}

func (*AppStack) PreDelete(_ *session.Session) error {
	return nil
}

func (*AppStack) PostDelete(_ *session.Session, _ *string) error {
	return nil
}

func (a *AppStack) ClusterName() string {
	return strings.TrimPrefix(a.Parameters.ClusterStackName, fmt.Sprintf(clusterStackNameTmpl, ""))
}

func (a *AppStack) StackType() string {
	if a.Pipeline {
		return "pipeline"
	}
	return "app"
}

func (a *AppStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	err := ui.FlagsToStruct(a.Parameters, flags)
	if err != nil {
		return err
	}
	sort.Strings(a.Parameters.AllowedUsers)
	return nil
}

func (a *AppStack) AskForDatabase(sess *session.Session) error {
	enable := a.Parameters.DatabaseStackName != ""
	var helpText string
	if a.Pipeline {
		helpText = "Review apps will create databases on a database instance in the cluster. " +
			"See https://docs.apppack.io/how-to/using-databases/ for more info."
	} else {
		helpText = "Create a database for the app on a database instance in the cluster. " +
			"Answering yes will create a user and database and provide the credentials to the app as a config variable. " +
			"See https://docs.apppack.io/how-to/using-databases/ for more info."
	}
	defaultValue := ui.BooleanAsYesNo(enable)
	var selected = defaultValue
	err := ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  fmt.Sprintf("Should a database be created for this %s?", a.StackType()),
			HelpText: helpText,
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Database").
						Options(huh.NewOptions("yes", "no")...).
						Value(&selected),
				),
			),
		},
	}, a.Parameters)
	if err != nil {
		return err
	}
	// Convert selection back to boolean
	enable = (selected == "yes")

	if enable {
		canChange, err := a.CanChangeParameter("DatabaseStackName")
		if err != nil {
			return err
		}
		if canChange {
			return a.AskForDatabaseStack(sess)
		}
		return nil
	}
	a.Parameters.DatabaseStackName = ""
	return nil
}

// DatabaseStackParameters converts `{name} ({Engine})` -> `{stackName}`
func databaseSelectTransform(value string) string {
	if value != "" {
		parts := strings.Split(value, " ")
		return fmt.Sprintf(databaseStackNameTmpl, parts[0])
	}
	return value
}

// AskForDatabaseStack gives the user a choice of available database stacks
func (a *AppStack) AskForDatabaseStack(sess *session.Session) error {
	clusterName := a.ClusterName()
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

	if a.Parameters.DatabaseStackName != "" {
		for i, db := range databases {
			name := strings.Split(db, " ")[0]
			if fmt.Sprintf(databaseStackNameTmpl, name) == a.Parameters.DatabaseStackName {
				defaultDatabaseIdx = i

				break
			}
		}
	}
	var verbose string
	if a.Pipeline {
		verbose = "Which database cluster should this pipeline's review app databases be setup on?"
	} else {
		verbose = "Which database cluster should this app's database be setup on?"
	}
	var selectedDatabase = databases[defaultDatabaseIdx]
	err = ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose: verbose,
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Database Cluster").
						Options(huh.NewOptions(databases...)...).
						Value(&selectedDatabase),
				),
			),
		},
	}, a.Parameters)
	if err == nil {
		a.Parameters.DatabaseStackName = databaseSelectTransform(selectedDatabase)
	}
	if err != nil {
		return err
	}
	return nil
}

// RedisStackParameters converts `{name}` -> `{stackName}`
func redisSelectTransform(value string) string {
	if value != "" {
		return fmt.Sprintf(redisStackNameTmpl, value)
	}
	return value
}

func (a *AppStack) AskForRedis(sess *session.Session) error {
	enable := a.Parameters.RedisStackName != ""
	var verbose string
	var helpText string
	if a.Pipeline {
		verbose = "Should review apps on this pipeline have access to a Redis database?"
		helpText = "Create a Redis user for the review apps on this pipeline on a Redis instance in the cluster. " +
			"See https://docs.apppack.io/how-to/using-redis/ for more info."
	} else {
		verbose = "Should this app have access to a Redis database?"
		helpText = "Create a Redis user for the app on a Redis instance in the cluster. " +
			"Answering yes will create a user and provide the credentials to the app as a config variable. " +
			"See https://docs.apppack.io/how-to/using-redis/ for more info."
	}
	var redisSel = ui.BooleanAsYesNo(enable)
	err := ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  verbose,
			HelpText: helpText,
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Redis").
						Options(huh.NewOptions("yes", "no")...).
						Value(&redisSel),
				),
			),
		},
	}, a.Parameters)
	if err != nil {
		return err
	}
	// Convert selection back to boolean
	enable = (redisSel == "yes")
	if enable {
		canChange, err := a.CanChangeParameter("RedisStackName")
		if err != nil {
			return err
		}
		if canChange {
			return a.AskForRedisStack(sess)
		}
		return nil
	}
	a.Parameters.RedisStackName = ""
	return nil
}

// AskForRedisStack gives the user a choice of available Redis stacks
func (a *AppStack) AskForRedisStack(sess *session.Session) error {
	clusterName := a.ClusterName()
	redises, err := ddb.ListStacks(sess, &clusterName, "REDIS")
	if err != nil {
		return err
	}
	if len(redises) == 0 {
		return fmt.Errorf("no AppPack Redis instances are setup on %s cluster", clusterName)
	}
	// set the current database as default
	defaultRedisIdx := 0

	if a.Parameters.RedisStackName != "" {
		for i, r := range redises {
			if fmt.Sprintf(databaseStackNameTmpl, r) == a.Parameters.RedisStackName {
				defaultRedisIdx = i

				break
			}
		}
	}
	var verbose string
	if a.Pipeline {
		verbose = "Which Redis instance should this pipeline's review apps be setup on?"
	} else {
		verbose = "Which Redis instance should this app's user be setup on?"
	}
	var selectedRedis = redises[defaultRedisIdx]
	err = ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose: verbose,
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Redis Cluster").
						Options(huh.NewOptions(redises...)...).
						Value(&selectedRedis),
				),
			),
		},
	}, a.Parameters)
	if err == nil {
		a.Parameters.RedisStackName = redisSelectTransform(selectedRedis)
	}
	if err != nil {
		return err
	}
	return nil
}

func (a *AppStack) AskForSES() error {
	enable := a.Parameters.SesDomain != ""
	var verbose string
	var helpText string
	if a.Pipeline {
		verbose = "Should review apps on this pipeline be allowed to send email via Amazon SES?"
		helpText = "Allow this pipeline's review apps to send email via SES. See https://docs.apppack.io/how-to/sending-mail/ for more info."
	} else {
		verbose = "Should this app be allowed to send email via Amazon SES?"
		helpText = "Allow this app to send email via SES. See https://docs.apppack.io/how-to/sending-email/ for more info."
	}
	var sesSel = ui.BooleanAsYesNo(enable)
	err := ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  verbose,
			HelpText: helpText,
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("SES (email)").
						Options(huh.NewOptions("yes", "no")...).
						Value(&sesSel),
				),
			),
		},
	}, a.Parameters)
	if err != nil {
		return err
	}
	// Convert selection back to boolean
	enable = (sesSel == "yes")
	if enable {
		return a.AskForSESDomain()
	}
	a.Parameters.SesDomain = ""
	return nil
}

// AskForRedisStack gives the user a choice of available Redis stacks
func (a *AppStack) AskForSESDomain() error {
	var verbose string
	if a.Pipeline {
		verbose = "Which domain should this pipeline's review apps be allowed to send from?"
	} else {
		verbose = "Which domain should this app be allowed to send from?"
	}
	err := ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  verbose,
			HelpText: "Only allow outbound email via SES from a specific domain (e.g., example.com). Use `*` to allow sending on any domain approved for sending in SES.",
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("SES Approved Domain").
						Placeholder(a.Parameters.SesDomain).
						Value(&a.Parameters.SesDomain),
				),
			),
		},
	}, a.Parameters)
	if err != nil {
		return err
	}
	return nil
}

// CanChangeParameter prevents users from changing stateful parameters
func (a *AppStack) CanChangeParameter(name string) (bool, error) {
	// stack isn't created yet
	if a.Stack == nil {
		return true, nil
	}
	currentVal, err := bridge.GetStackParameter(a.Stack.Parameters, name)
	if err != nil {
		return false, err
	}
	// no database is set
	return *currentVal == "", nil
}

func (a *AppStack) AskQuestions(sess *session.Session) error { // skipcq: GO-R1005
	var questions []*ui.QuestionExtra
	var err error
	if a.Stack == nil {
		err = AskForCluster(
			sess,
			fmt.Sprintf("Which cluster should this %s be installed in?", a.StackType()),
			"A cluster represents an isolated network and its associated resources (Database, Redis, etc.).",
			a.Parameters,
		)
		if err != nil {
			return err
		}
	}
	sort.Strings(a.Parameters.AllowedUsers)
	questions = append(questions, &ui.QuestionExtra{
		Verbose:  fmt.Sprintf("What code repository should this %s build from?", a.StackType()),
		HelpText: "Use the HTTP URL (e.g., https://github.com/{org}/{repo}.git). BitBucket and Github repositories are supported.",
		Form: huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Repository URL").
					Placeholder(a.Parameters.RepositoryUrl).
					Value(&a.Parameters.RepositoryUrl),
			),
		),
	})
	if err = ui.AskQuestions(questions, a.Parameters); err != nil {
		return err
	}
	questions = []*ui.QuestionExtra{}
	if err := a.Parameters.SetRepositoryType(); err != nil {
		return err
	}
	if err = verifySourceCredentials(sess, a.Parameters.RepositoryType); err != nil {
		return err
	}
	if !a.Pipeline {
		var domainText = strings.Join(a.Parameters.Domains, "\n")
		questions = append(questions, []*ui.QuestionExtra{
			{
				Verbose:  "What branch should this app build from?",
				HelpText: "The deployment pipeline will be triggered on new pushes to this branch.",
				Form: huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Branch").
							Placeholder(a.Parameters.Branch).
							Value(&a.Parameters.Branch),
					),
				),
			},
			{
				Verbose:  "Should the app be served on a custom domain? (Optional)",
				HelpText: "By default, the app will automatically be assigned a domain within the cluster. If you'd like it to respond on other domain(s), enter them here (one-per-line). See https://docs.apppack.io/how-to/custom-domains/ for more info.",
				Form: huh.NewForm(
					huh.NewGroup(
						huh.NewText().
							Title("Custom Domain(s)").
							Placeholder(domainText).
							Value(&domainText),
					),
				),
			},
		}...)
		// Convert domainText back to slice
		if domainText != "" {
			a.Parameters.Domains = strings.Split(domainText, "\n")
		} else {
			a.Parameters.Domains = []string{}
		}
	}
	var sqsVerbose string
	var sqsHelpText string
	var bucketHelpTextApp string
	if a.Pipeline {
		sqsVerbose = "Should an SQS Queue be created for review apps on this pipeline?"
		sqsHelpText = "The SQS Queue can be used to queue up messages between processes. Answering yes will create the queue for each review app and provide its name to the app as a config variable. See https://docs.apppack.io/how-to/using-sqs/ for more info."
		bucketHelpTextApp = "review apps"
	} else {
		sqsVerbose = "Should an SQS Queue be created for this app?"
		sqsHelpText = "The SQS Queue can be used to queue up messages between processes. Answering yes will create the queue and provide its name to the app as a config variable. See https://docs.apppack.io/how-to/using-sqs/ for more info."
		bucketHelpTextApp = "the app"
	}

	// Variables for boolean selections
	var privateS3Sel = ui.BooleanAsYesNo(a.Parameters.PrivateS3BucketEnabled)
	var publicS3Sel = ui.BooleanAsYesNo(a.Parameters.PublicS3BucketEnabled)
	var sqsSel = ui.BooleanAsYesNo(a.Parameters.SQSQueueEnabled)

	questions = append(questions, []*ui.QuestionExtra{
		{
			Verbose:  "What path should be used for healthchecks?",
			HelpText: "Enter a path (e.g., `/-/alive/`) that will always serve a 200 status code when the application is healthy.",
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Healthcheck Path").
						Placeholder(a.Parameters.HealthCheckPath).
						Value(&a.Parameters.HealthCheckPath),
				),
			),
		},
		{
			Verbose:  fmt.Sprintf("Should a private S3 Bucket be created for this %s?", a.StackType()),
			HelpText: fmt.Sprintf("The S3 Bucket can be used to store files that should not be publicly accessible. Answering yes will create the bucket and provide its name to %s as a config variable. See https://docs.apppack.io/how-to/using-s3/ for more info.", bucketHelpTextApp),
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Private S3 Bucket").
						Options(huh.NewOptions("yes", "no")...).
						Value(&privateS3Sel),
				),
			),
		},
		{
			Verbose:  fmt.Sprintf("Should a public S3 Bucket be created for this %s?", a.StackType()),
			HelpText: fmt.Sprintf("The S3 Bucket can be used to store files that should not be publicly accessible. Answering yes will create the bucket and provide its name to %s as a config variable. See https://docs.apppack.io/how-to/using-s3/ for more info.", bucketHelpTextApp),
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Public S3 Bucket").
						Options(huh.NewOptions("yes", "no")...).
						Value(&publicS3Sel),
				),
			),
		},
		{
			Verbose:  sqsVerbose,
			HelpText: sqsHelpText,
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("SQS Queue").
						Options(huh.NewOptions("yes", "no")...).
						Value(&sqsSel),
				),
			),
		},
	}...)
	if err = ui.AskQuestions(questions, a.Parameters); err != nil {
		return err
	}
	// Convert selections back to booleans
	a.Parameters.PrivateS3BucketEnabled = (privateS3Sel == "yes")
	a.Parameters.PublicS3BucketEnabled = (publicS3Sel == "yes")
	a.Parameters.SQSQueueEnabled = (sqsSel == "yes")
	if err := a.AskForDatabase(sess); err != nil {
		return err
	}
	if err := a.AskForRedis(sess); err != nil {
		return err
	}
	if err := a.AskForSES(); err != nil {
		return err
	}
	if a.Stack == nil {
		var usersText = strings.Join(a.Parameters.AllowedUsers, "\n")
		err = ui.AskQuestions([]*ui.QuestionExtra{
			{
				Verbose:  fmt.Sprintf("Who can manage this %s?", a.StackType()),
				HelpText: fmt.Sprintf("A list of email addresses (one per line) who have access to manage this %s via AppPack.", a.StackType()),
				Form: huh.NewForm(
					huh.NewGroup(
						huh.NewText().
							Title("Users").
							Value(&usersText),
					),
				),
			},
		}, a.Parameters)
		if err != nil {
			return err
		}
		// Convert usersText back to slice
		if usersText != "" {
			a.Parameters.AllowedUsers = strings.Split(usersText, "\n")
		} else {
			a.Parameters.AllowedUsers = []string{}
		}
	} else if err = a.WarnIfDataLoss(); err != nil {
		return err
	}

	return nil
}

func (a *AppStack) WarnIfDataLoss() error {
	fmt.Println()
	privateS3BucketDestroy, err := a.PrivateS3BucketToBeDestroyed()
	if err != nil {
		return err
	}
	publicS3BucketDestroy, err := a.PublicS3BucketToBeDestroyed()
	if err != nil {
		return err
	}
	databaseDestroy, err := a.DatabaseToBeDestroyed()
	if err != nil {
		return err
	}
	redisDestroy, err := a.RedisToBeDestroyed()
	if err != nil {
		return err
	}
	if privateS3BucketDestroy {
		ui.PrintWarning("The current private S3 Bucket and all files in it will be permanently destroyed.")
	}
	if publicS3BucketDestroy {
		ui.PrintWarning("The current public S3 Bucket and all files in it will be permanently destroyed.")
	}
	if databaseDestroy {
		ui.PrintWarning("The current app database and all data in it will be permanently destroyed.")
	}
	if redisDestroy {
		ui.PrintWarning("The current Redis database will no longer be accessible to the application.")
	}
	if privateS3BucketDestroy || publicS3BucketDestroy || databaseDestroy || redisDestroy {
		var verify = "no"
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Are you sure you want to continue?").
					Options(huh.NewOptions("yes", "no")...).
					Value(&verify),
			),
		)
		if err := form.Run(); err != nil {
			return err
		}
		if verify != "yes" {
			return fmt.Errorf("aborted due to user input")
		}
	}
	return nil
}

func (a *AppStack) PublicS3BucketToBeDestroyed() (bool, error) {
	val, err := bridge.GetStackParameter(a.Stack.Parameters, "PublicS3BucketEnabled")
	if err != nil {
		return false, err
	}
	return *val == Enabled && !a.Parameters.PublicS3BucketEnabled, nil
}

func (a *AppStack) PrivateS3BucketToBeDestroyed() (bool, error) {
	val, err := bridge.GetStackParameter(a.Stack.Parameters, "PrivateS3BucketEnabled")
	if err != nil {
		return false, err
	}
	return *val == Enabled && !a.Parameters.PrivateS3BucketEnabled, nil
}

func (a *AppStack) DatabaseToBeDestroyed() (bool, error) {
	val, err := bridge.GetStackParameter(a.Stack.Parameters, "DatabaseStackName")
	if err != nil {
		return false, err
	}
	return *val != "" && *val != a.Parameters.DatabaseStackName, nil
}

func (a *AppStack) RedisToBeDestroyed() (bool, error) {
	val, err := bridge.GetStackParameter(a.Stack.Parameters, "RedisStackName")
	if err != nil {
		return false, err
	}
	return *val != "" && *val != a.Parameters.RedisStackName, nil
}

func (a *AppStack) StackName(name *string) *string {
	var stackName string
	if a.Pipeline {
		stackName = fmt.Sprintf(PipelineStackNameTmpl, *name)
	} else {
		stackName = fmt.Sprintf(AppStackNameTmpl, *name)
	}
	return &stackName
}

func (a *AppStack) Tags(name *string) []*cloudformation.Tag {
	tags := []*cloudformation.Tag{
		{Key: aws.String("apppack:appName"), Value: name},
		{Key: aws.String("apppack:cluster"), Value: aws.String(a.ClusterName())},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
	if a.Pipeline {
		tags = append(tags, &cloudformation.Tag{
			Key:   aws.String("apppack:pipeline"),
			Value: aws.String("true"),
		})
	}
	return tags
}

func (*AppStack) Capabilities() []*string {
	return []*string{
		aws.String("CAPABILITY_IAM"),
	}
}

func (*AppStack) TemplateURL(release *string) *string {
	url := appFormationURL
	if release != nil {
		url = strings.Replace(appFormationURL, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}
	return &url
}

func verifySourceCredentials(sess *session.Session, repositoryType string) error {
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
		} else if repositoryType == "GITHUB" {
			friendlySourceName = "GitHub"
		} else {
			return fmt.Errorf("unsupported repository type: %s", repositoryType)
		}
		ui.Spinner.Stop()
		ui.PrintWarning(fmt.Sprintf("CodeBuild needs to be authenticated to access your repository at %s", friendlySourceName))
		fmt.Println("On the CodeBuild new project page:")
		fmt.Printf("    1. Scroll to the %s section\n", aurora.Bold("Source"))
		fmt.Printf("    2. Select %s for the %s\n", aurora.Bold(friendlySourceName), aurora.Bold("Source provider"))
		fmt.Printf("    3. Keep the default %s\n", aurora.Bold("Connect using OAuth"))
		fmt.Printf("    4. Click %s\n", aurora.Bold(fmt.Sprintf("Connect to %s", friendlySourceName)))
		fmt.Printf("    5. Click %s in the popup window\n", aurora.Bold("Confirm"))
		fmt.Printf("    6. %s You can close the browser window and continue with app setup here.\n\n", aurora.Bold("That's it!"))
		newProjectURL := fmt.Sprintf("https://%s.console.aws.amazon.com/codesuite/codebuild/project/new", *sess.Config.Region)
		url, err := auth.GetConsoleURL(sess, newProjectURL)
		if err == nil && isatty.IsTerminal(os.Stdin.Fd()) {
			fmt.Println("Opening the CodeBuild new project page now...")
			err = browser.OpenURL(*url)
			if err != nil {
				fmt.Println("Open this URL in your browser to view logs:")
				fmt.Println(*url)
			}
		} else {
			fmt.Printf("Visit the following URL to authenticate: %s", newProjectURL)
		}
		ui.PauseUntilEnter("Finish authentication in your web browser then press ENTER to continue.")
		return verifySourceCredentials(sess, repositoryType)
	}
	return nil
}

func GetPipelineStack(sess *session.Session, name string) (*cloudformation.Stack, error) {
	return bridge.GetStack(sess, fmt.Sprintf(PipelineStackNameTmpl, name))
}
