package stacks

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"

	"github.com/apppackio/apppack/auth"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ddb"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	codebuildtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/charmbracelet/huh"
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
	RepositoryURL                      string   `flag:"repository" cfnparam:"RepositoryUrl"`
	Branch                             string   `flag:"branch"`
	Domains                            []string `flag:"domains"`
	DefaultAutoscalingAverageCPUTarget int      `cfnparam:"DefaultAutoscalingAverageCpuTarget"`
	HealthCheckPath                    string   `flag:"healthcheck-path"`
	HealthcheckInterval                int
	DeregistrationDelay                int
	LoadBalancerRulePriority           int
	LogRetentionDays                   int
	AppPackRoleExternalID              string `cfnparam:"AppPackRoleExternalId"`
	PrivateS3BucketEnabled             bool   `flag:"addon-private-s3"`
	PublicS3BucketEnabled              bool   `flag:"addon-public-s3"`
	SESDomain                          string `flag:"addon-ses-domain" cfnparam:"SesDomain"`
	DatabaseStackName                  string `flag:"addon-database-name;fmtString:apppack-database-%s"`
	RedisStackName                     string `flag:"addon-redis-name;fmtString:apppack-redis-%s"`
	SQSQueueEnabled                    bool   `flag:"addon-sqs"`
	RepositoryType                     string
	Fargate                            bool     `flag:"ec2;negate"`
	AllowedUsers                       []string `flag:"users"`
	BuildWebhook                       bool     `flag:"disable-build-webhook;negate"`
	CustomTaskPolicyARN                string   `cfnparam:"CustomTaskPolicyArn"`
}

var DefaultAppStackParameters = AppStackParameters{
	Type:                               "app",
	HealthCheckPath:                    "/",
	HealthcheckInterval:                30,
	LogRetentionDays:                   30,
	DefaultAutoscalingAverageCPUTarget: 50,
	DeregistrationDelay:                15,
	Fargate:                            true,
	BuildWebhook:                       true,
}

var DefaultPipelineStackParameters = AppStackParameters{
	Type:                               "pipeline",
	HealthCheckPath:                    DefaultAppStackParameters.HealthCheckPath,
	HealthcheckInterval:                DefaultAppStackParameters.HealthcheckInterval,
	LogRetentionDays:                   DefaultAppStackParameters.LogRetentionDays,
	DefaultAutoscalingAverageCPUTarget: DefaultAppStackParameters.DefaultAutoscalingAverageCPUTarget,
	DeregistrationDelay:                DefaultAppStackParameters.DeregistrationDelay,
	Fargate:                            DefaultAppStackParameters.Fargate,
	BuildWebhook:                       DefaultAppStackParameters.BuildWebhook,
}

func (p *AppStackParameters) Import(parameters []types.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *AppStackParameters) ToCloudFormationParameters() ([]types.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *AppStackParameters) SetInternalFields(_ aws.Config, name *string) error {
	// update values from flags if they are set
	if p.LoadBalancerRulePriority == 0 {
		p.LoadBalancerRulePriority = rand.Intn(50000-200) + 200 // #nosec G404 -- Non-crypto random for LB priority assignment
	}

	if err := p.SetRepositoryType(); err != nil {
		return err
	}

	if p.AppPackRoleExternalID == "" {
		// TODO: This should come from us instead of the user
		p.AppPackRoleExternalID = strings.ReplaceAll(uuid.New().String(), "-", "")
	}

	if p.Name == "" {
		p.Name = *name
	}

	return nil
}

func (p *AppStackParameters) SetRepositoryType() error {
	if strings.Contains(p.RepositoryURL, "github.com") {
		p.RepositoryType = "GITHUB"

		return nil
	}

	if strings.Contains(p.RepositoryURL, "bitbucket.org") {
		p.RepositoryType = "BITBUCKET"

		return nil
	}

	return errors.New("unknown repository source")
}

type AppStack struct {
	Stack      *types.Stack
	Parameters *AppStackParameters
	Pipeline   bool
}

func (a *AppStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *AppStack) GetStack() *types.Stack {
	return a.Stack
}

func (a *AppStack) SetStack(stack *types.Stack) {
	a.Stack = stack
}

func (*AppStack) PostCreate(_ aws.Config) error {
	return nil
}

func (*AppStack) PreDelete(_ aws.Config) error {
	return nil
}

func (*AppStack) PostDelete(_ aws.Config, _ *string) error {
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

func (a *AppStack) AskForDatabase(cfg aws.Config) error {
	enable := a.Parameters.DatabaseStackName != ""

	var verbose string
	var helpText string
	if a.Pipeline {
		verbose = fmt.Sprintf("Should a database be created for this %s?", a.StackType())
		helpText = "Review apps will create databases on a database instance in the cluster. " +
			"See https://docs.apppack.io/how-to/using-databases/ for more info."
	} else {
		verbose = fmt.Sprintf("Should a database be created for this %s?", a.StackType())
		helpText = "Create a database for the app on a database instance in the cluster. " +
			"Answering yes will create a user and database and provide the credentials to the app as a config variable. " +
			"See https://docs.apppack.io/how-to/using-databases/ for more info."
	}

	form, selectedPtr := AppDatabaseForm(verbose, helpText, enable)
	if err := form.Run(); err != nil {
		return err
	}

	enable = ui.YesNoToBool(*selectedPtr)

	if enable {
		canChange, err := a.CanChangeParameter("DatabaseStackName")
		if err != nil {
			return err
		}

		if canChange {
			return a.AskForDatabaseStack(cfg)
		}

		return nil
	}

	a.Parameters.DatabaseStackName = ""

	return nil
}

// AskForDatabaseStack gives the user a choice of available database stacks
func (a *AppStack) AskForDatabaseStack(cfg aws.Config) error {
	clusterName := a.ClusterName()
	// databases is a list of `{name} ({engine})` for the databases in the cluster
	databases, err := ddb.ListStacks(cfg, &clusterName, "DATABASE")
	if err != nil {
		return err
	}

	if len(databases) == 0 {
		return fmt.Errorf("no AppPack databases are setup on %s cluster", clusterName)
	}

	// Build typed options: display is "{name} ({engine})", value is the full stack name.
	options := make([]huh.Option[string], len(databases))
	for i, db := range databases {
		parts := strings.Split(db, " ")
		stackName := fmt.Sprintf(databaseStackNameTmpl, parts[0])
		opt := huh.NewOption(db, stackName)
		if stackName == a.Parameters.DatabaseStackName {
			opt = opt.Selected(true)
		}
		options[i] = opt
	}

	var verbose string
	if a.Pipeline {
		verbose = "Which database cluster should this pipeline's review app databases be setup on?"
	} else {
		verbose = "Which database cluster should this app's database be setup on?"
	}

	form, selectedPtr := AppDatabaseStackForm(options, verbose)
	if err := form.Run(); err != nil {
		return err
	}

	a.Parameters.DatabaseStackName = *selectedPtr

	return nil
}

func (a *AppStack) AskForRedis(cfg aws.Config) error {
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

	form, selectedPtr := AppRedisForm(verbose, helpText, enable)
	if err := form.Run(); err != nil {
		return err
	}

	enable = ui.YesNoToBool(*selectedPtr)

	if enable {
		canChange, err := a.CanChangeParameter("RedisStackName")
		if err != nil {
			return err
		}

		if canChange {
			return a.AskForRedisStack(cfg)
		}

		return nil
	}

	a.Parameters.RedisStackName = ""

	return nil
}

// AskForRedisStack gives the user a choice of available Redis stacks
func (a *AppStack) AskForRedisStack(cfg aws.Config) error {
	clusterName := a.ClusterName()

	redises, err := ddb.ListStacks(cfg, &clusterName, "REDIS")
	if err != nil {
		return err
	}

	if len(redises) == 0 {
		return fmt.Errorf("no AppPack Redis instances are setup on %s cluster", clusterName)
	}

	// Build typed options: display is the Redis name, value is the full stack name.
	options := make([]huh.Option[string], len(redises))
	for i, r := range redises {
		stackName := fmt.Sprintf(redisStackNameTmpl, r)
		opt := huh.NewOption(r, stackName)
		if stackName == a.Parameters.RedisStackName {
			opt = opt.Selected(true)
		}
		options[i] = opt
	}

	var verbose string
	if a.Pipeline {
		verbose = "Which Redis instance should this pipeline's review apps be setup on?"
	} else {
		verbose = "Which Redis instance should this app's user be setup on?"
	}

	form, selectedPtr := AppRedisStackForm(options, verbose)
	if err := form.Run(); err != nil {
		return err
	}

	a.Parameters.RedisStackName = *selectedPtr

	return nil
}

func (a *AppStack) AskForSES() error {
	enable := a.Parameters.SESDomain != ""

	var verbose string
	var helpText string

	if a.Pipeline {
		verbose = "Should review apps on this pipeline be allowed to send email via Amazon SES?"
		helpText = "Allow this pipeline's review apps to send email via SES. See https://docs.apppack.io/how-to/sending-mail/ for more info."
	} else {
		verbose = "Should this app be allowed to send email via Amazon SES?"
		helpText = "Allow this app to send email via SES. See https://docs.apppack.io/how-to/sending-email/ for more info."
	}

	form, selectedPtr := AppSESForm(verbose, helpText, enable)
	if err := form.Run(); err != nil {
		return err
	}

	if ui.YesNoToBool(*selectedPtr) {
		return a.AskForSESDomain()
	}

	a.Parameters.SESDomain = ""

	return nil
}

// AskForSESDomain prompts the user to enter the SES approved domain
func (a *AppStack) AskForSESDomain() error {
	var verbose string
	if a.Pipeline {
		verbose = "Which domain should this pipeline's review apps be allowed to send from?"
	} else {
		verbose = "Which domain should this app be allowed to send from?"
	}

	form, domainPtr := AppSESDomainForm(verbose, a.Parameters.SESDomain)
	if err := form.Run(); err != nil {
		return err
	}

	a.Parameters.SESDomain = *domainPtr

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

// AppRepositoryURLForm builds the interactive form for entering the repository URL.
// Returns the form and a pointer to the entered URL value.
func AppRepositoryURLForm(defaultURL string) (*huh.Form, *string) {
	url := defaultURL

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("What code repository should this app build from?").
				Description("Use the HTTP URL (e.g., https://github.com/{org}/{repo}.git). BitBucket and Github repositories are supported."),
			huh.NewInput().
				Title("Repository URL").
				Value(&url).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("repository URL is required")
					}

					return nil
				}),
		),
	)

	return form, &url
}

// AppBranchForm builds the interactive form for entering the deployment branch.
// Returns the form and a pointer to the entered branch value.
func AppBranchForm(defaultBranch string) (*huh.Form, *string) {
	branch := defaultBranch

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("What branch should this app build from?").
				Description("The deployment pipeline will be triggered on new pushes to this branch."),
			huh.NewInput().
				Title("Branch").
				Value(&branch).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("branch is required")
					}

					return nil
				}),
		),
	)

	return form, &branch
}

// AppDomainsForm builds the interactive form for entering custom domains.
// Returns the form and a pointer to the raw newline-separated domains string.
func AppDomainsForm(defaultDomains []string) (*huh.Form, *string) {
	domainsStr := strings.Join(defaultDomains, "\n")

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Should the app be served on a custom domain? (Optional)").
				Description("By default, the app will automatically be assigned a domain within the cluster. If you'd like it to respond on other domain(s), enter them here (one-per-line). See https://docs.apppack.io/how-to/custom-domains/ for more info."),
			huh.NewText().
				Title("Custom Domain(s)").
				Value(&domainsStr).
				Validate(func(s string) error {
					if s == "" {
						return nil
					}

					domains := strings.Split(s, "\n")
					if len(domains) > 4 {
						return errors.New("limit of 4 custom domains exceeded")
					}

					return nil
				}),
		),
	)

	return form, &domainsStr
}

// AppHealthCheckPathForm builds the interactive form for entering the health check path.
// Returns the form and a pointer to the entered path value.
func AppHealthCheckPathForm(defaultPath string) (*huh.Form, *string) {
	path := defaultPath

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("What path should be used for healthchecks?").
				Description("Enter a path (e.g., `/-/alive/`) that will always serve a 200 status code when the application is healthy."),
			huh.NewInput().
				Title("Healthcheck Path").
				Value(&path).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("healthcheck path is required")
					}

					return nil
				}),
		),
	)

	return form, &path
}

// AppPrivateS3Form builds the interactive form for enabling/disabling a private S3 bucket.
// Returns the form and a pointer to the selected "yes"/"no" value.
func AppPrivateS3Form(verbose, helpText string, defaultEnabled bool) (*huh.Form, *string) {
	selected := ui.BooleanAsYesNo(defaultEnabled)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose).
				Description(helpText),
			huh.NewSelect[string]().
				Title("Private S3 Bucket").
				Options(ui.YesNoOptions(defaultEnabled)...).
				Value(&selected),
		),
	)

	return form, &selected
}

// AppPublicS3Form builds the interactive form for enabling/disabling a public S3 bucket.
// Returns the form and a pointer to the selected "yes"/"no" value.
func AppPublicS3Form(verbose, helpText string, defaultEnabled bool) (*huh.Form, *string) {
	selected := ui.BooleanAsYesNo(defaultEnabled)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose).
				Description(helpText),
			huh.NewSelect[string]().
				Title("Public S3 Bucket").
				Options(ui.YesNoOptions(defaultEnabled)...).
				Value(&selected),
		),
	)

	return form, &selected
}

// AppSQSForm builds the interactive form for enabling/disabling an SQS queue.
// Returns the form and a pointer to the selected "yes"/"no" value.
func AppSQSForm(verbose, helpText string, defaultEnabled bool) (*huh.Form, *string) {
	selected := ui.BooleanAsYesNo(defaultEnabled)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose).
				Description(helpText),
			huh.NewSelect[string]().
				Title("SQS Queue").
				Options(ui.YesNoOptions(defaultEnabled)...).
				Value(&selected),
		),
	)

	return form, &selected
}

// AppDatabaseForm builds the interactive yes/no form for enabling/disabling a database.
// Returns the form and a pointer to the selected "yes"/"no" value.
func AppDatabaseForm(verbose, helpText string, defaultEnabled bool) (*huh.Form, *string) {
	selected := ui.BooleanAsYesNo(defaultEnabled)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose).
				Description(helpText),
			huh.NewSelect[string]().
				Title("Database").
				Options(ui.YesNoOptions(defaultEnabled)...).
				Value(&selected),
		),
	)

	return form, &selected
}

// AppDatabaseStackForm builds the interactive form for selecting a database stack.
// Returns the form and a pointer to the selected stack name value.
func AppDatabaseStackForm(options []huh.Option[string], verbose string) (*huh.Form, *string) {
	var selected string
	if len(options) > 0 {
		// Pre-seed with the first option value; actual selection handled by Selected() on options.
		selected = options[0].Value
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose),
			huh.NewSelect[string]().
				Title("Database Cluster").
				Options(options...).
				Value(&selected),
		),
	)

	return form, &selected
}

// AppRedisForm builds the interactive yes/no form for enabling/disabling Redis.
// Returns the form and a pointer to the selected "yes"/"no" value.
func AppRedisForm(verbose, helpText string, defaultEnabled bool) (*huh.Form, *string) {
	selected := ui.BooleanAsYesNo(defaultEnabled)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose).
				Description(helpText),
			huh.NewSelect[string]().
				Title("Redis").
				Options(ui.YesNoOptions(defaultEnabled)...).
				Value(&selected),
		),
	)

	return form, &selected
}

// AppRedisStackForm builds the interactive form for selecting a Redis stack.
// Returns the form and a pointer to the selected stack name value.
func AppRedisStackForm(options []huh.Option[string], verbose string) (*huh.Form, *string) {
	var selected string
	if len(options) > 0 {
		selected = options[0].Value
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose),
			huh.NewSelect[string]().
				Title("Redis Cluster").
				Options(options...).
				Value(&selected),
		),
	)

	return form, &selected
}

// AppSESForm builds the interactive yes/no form for enabling/disabling SES email.
// Returns the form and a pointer to the selected "yes"/"no" value.
func AppSESForm(verbose, helpText string, defaultEnabled bool) (*huh.Form, *string) {
	selected := ui.BooleanAsYesNo(defaultEnabled)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose).
				Description(helpText),
			huh.NewSelect[string]().
				Title("SES (email)").
				Options(ui.YesNoOptions(defaultEnabled)...).
				Value(&selected),
		),
	)

	return form, &selected
}

// AppSESDomainForm builds the interactive form for entering the SES approved domain.
// Returns the form and a pointer to the entered domain value.
func AppSESDomainForm(verbose, defaultDomain string) (*huh.Form, *string) {
	domain := defaultDomain

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(verbose).
				Description("Only allow outbound email via SES from a specific domain (e.g., example.com). Use `*` to allow sending on any domain approved for sending in SES."),
			huh.NewInput().
				Title("SES Approved Domain").
				Value(&domain).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("SES domain is required")
					}

					return nil
				}),
		),
	)

	return form, &domain
}

// AppUsersForm builds the interactive form for entering allowed users (one per line).
// Returns the form and a pointer to the raw newline-separated users string.
func AppUsersForm(stackType string) (*huh.Form, *string) {
	var users string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("Who can manage this %s?", stackType)).
				Description(fmt.Sprintf("A list of email addresses (one per line) who have access to manage this %s via AppPack.", stackType)),
			huh.NewText().
				Title("Users").
				Value(&users).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("at least one user email is required")
					}

					return nil
				}),
		),
	)

	return form, &users
}

// AppDataLossConfirmForm builds the confirmation form displayed when a stack update
// would result in permanent data loss.
// Returns the form and a pointer to the confirmed bool value.
func AppDataLossConfirmForm() (*huh.Form, *bool) {
	confirmed := false

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Are you sure you want to continue?").
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	)

	return form, &confirmed
}

func (a *AppStack) AskQuestions(cfg aws.Config) error { // skipcq: GO-R1005
	var err error
	if a.Stack == nil {
		err = AskForCluster(
			cfg,
			fmt.Sprintf("Which cluster should this %s be installed in?", a.StackType()),
			"A cluster represents an isolated network and its associated resources (Database, Redis, etc.).",
			&a.Parameters.ClusterStackName,
		)
		if err != nil {
			return err
		}
	}

	sort.Strings(a.Parameters.AllowedUsers)

	// Repository URL
	ui.Spinner.Stop()
	repoForm, repoPtr := AppRepositoryURLForm(a.Parameters.RepositoryURL)
	if err = repoForm.Run(); err != nil {
		return err
	}

	a.Parameters.RepositoryURL = *repoPtr

	if err := a.Parameters.SetRepositoryType(); err != nil {
		return err
	}

	if err = VerifySourceCredentials(cfg, a.Parameters.RepositoryType); err != nil {
		return err
	}

	if !a.Pipeline {
		// Branch
		branchForm, branchPtr := AppBranchForm(a.Parameters.Branch)
		if err = branchForm.Run(); err != nil {
			return err
		}

		a.Parameters.Branch = *branchPtr

		// Custom domains
		domainsForm, domainsPtr := AppDomainsForm(a.Parameters.Domains)
		if err = domainsForm.Run(); err != nil {
			return err
		}

		a.Parameters.Domains = strings.Split(*domainsPtr, "\n")
	}

	// Healthcheck path
	healthForm, healthPtr := AppHealthCheckPathForm(a.Parameters.HealthCheckPath)
	if err = healthForm.Run(); err != nil {
		return err
	}

	a.Parameters.HealthCheckPath = *healthPtr

	// Private S3 bucket
	var bucketHelpTextApp string
	if a.Pipeline {
		bucketHelpTextApp = "review apps"
	} else {
		bucketHelpTextApp = "the app"
	}

	privateS3Form, privateS3Ptr := AppPrivateS3Form(
		fmt.Sprintf("Should a private S3 Bucket be created for this %s?", a.StackType()),
		fmt.Sprintf("The S3 Bucket can be used to store files that should not be publicly accessible. Answering yes will create the bucket and provide its name to %s as a config variable. See https://docs.apppack.io/how-to/using-s3/ for more info.", bucketHelpTextApp),
		a.Parameters.PrivateS3BucketEnabled,
	)
	if err = privateS3Form.Run(); err != nil {
		return err
	}

	a.Parameters.PrivateS3BucketEnabled = ui.YesNoToBool(*privateS3Ptr)

	// Public S3 bucket
	publicS3Form, publicS3Ptr := AppPublicS3Form(
		fmt.Sprintf("Should a public S3 Bucket be created for this %s?", a.StackType()),
		fmt.Sprintf("The S3 Bucket can be used to store files that should be publicly accessible. Answering yes will create the bucket and provide its name to %s as a config variable. See https://docs.apppack.io/how-to/using-s3/ for more info.", bucketHelpTextApp),
		a.Parameters.PublicS3BucketEnabled,
	)
	if err = publicS3Form.Run(); err != nil {
		return err
	}

	a.Parameters.PublicS3BucketEnabled = ui.YesNoToBool(*publicS3Ptr)

	// SQS queue
	var sqsVerbose, sqsHelpText string
	if a.Pipeline {
		sqsVerbose = "Should an SQS Queue be created for review apps on this pipeline?"
		sqsHelpText = "The SQS Queue can be used to queue up messages between processes. Answering yes will create the queue for each review app and provide its name to the app as a config variable. See https://docs.apppack.io/how-to/using-sqs/ for more info."
	} else {
		sqsVerbose = "Should an SQS Queue be created for this app?"
		sqsHelpText = "The SQS Queue can be used to queue up messages between processes. Answering yes will create the queue and provide its name to the app as a config variable. See https://docs.apppack.io/how-to/using-sqs/ for more info."
	}

	sqsForm, sqsPtr := AppSQSForm(sqsVerbose, sqsHelpText, a.Parameters.SQSQueueEnabled)
	if err = sqsForm.Run(); err != nil {
		return err
	}

	a.Parameters.SQSQueueEnabled = ui.YesNoToBool(*sqsPtr)

	if err := a.AskForDatabase(cfg); err != nil {
		return err
	}

	if err := a.AskForRedis(cfg); err != nil {
		return err
	}

	if err := a.AskForSES(); err != nil {
		return err
	}

	if a.Stack == nil {
		usersForm, usersPtr := AppUsersForm(a.StackType())
		if err = usersForm.Run(); err != nil {
			return err
		}

		a.Parameters.AllowedUsers = strings.Split(strings.TrimSpace(*usersPtr), "\n")
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
		form, confirmedPtr := AppDataLossConfirmForm()
		if err := form.Run(); err != nil {
			return err
		}

		if !*confirmedPtr {
			return errors.New("aborted due to user input")
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

func (a *AppStack) Tags(name *string) []types.Tag {
	tags := []types.Tag{
		{Key: aws.String("apppack:appName"), Value: name},
		{Key: aws.String("apppack:cluster"), Value: aws.String(a.ClusterName())},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
	if a.Pipeline {
		tags = append(tags, types.Tag{
			Key:   aws.String("apppack:pipeline"),
			Value: aws.String("true"),
		})
	}

	return tags
}

func (*AppStack) Capabilities() []types.Capability {
	return []types.Capability{
		types.CapabilityCapabilityIam,
	}
}

func (*AppStack) TemplateURL(release *string) *string {
	url := appFormationURL
	if release != nil {
		url = strings.Replace(appFormationURL, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}

	return &url
}

func VerifySourceCredentials(cfg aws.Config, repositoryType string) error {
	codebuildSvc := codebuild.NewFromConfig(cfg)

	sourceCredentialsOutput, err := codebuildSvc.ListSourceCredentials(context.Background(), &codebuild.ListSourceCredentialsInput{})
	if err != nil {
		return err
	}

	hasCredentials := false

	for _, cred := range sourceCredentialsOutput.SourceCredentialsInfos {
		if cred.ServerType == codebuildtypes.ServerType(repositoryType) {
			hasCredentials = true
		}
	}

	if !hasCredentials {
		var friendlySourceName string
		switch repositoryType {
		case "BITBUCKET":
			friendlySourceName = "Bitbucket"
		case "GITHUB":
			friendlySourceName = "GitHub"
		default:
			return fmt.Errorf("unsupported repository type: %s", repositoryType)
		}

		ui.Spinner.Stop()
		ui.PrintWarning("CodeBuild needs to be authenticated to access your repository at " + friendlySourceName)
		fmt.Println("On the CodeBuild new project page:")
		fmt.Printf("    1. Scroll to the %s section\n", aurora.Bold("Source"))
		fmt.Printf("    2. Select %s for the %s\n", aurora.Bold(friendlySourceName), aurora.Bold("Source provider"))
		fmt.Printf("    3. Keep the default %s\n", aurora.Bold("Connect using OAuth"))
		fmt.Printf("    4. Click %s\n", aurora.Bold("Connect to "+friendlySourceName))
		fmt.Printf("    5. Click %s in the popup window\n", aurora.Bold("Confirm"))
		fmt.Printf("    6. %s You can close the browser window and continue with app setup here.\n\n", aurora.Bold("That's it!"))
		newProjectURL := fmt.Sprintf("https://%s.console.aws.amazon.com/codesuite/codebuild/project/new", cfg.Region)

		url, err := auth.GetConsoleURL(cfg, newProjectURL)
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

		return VerifySourceCredentials(cfg, repositoryType)
	}

	return nil
}

func GetPipelineStack(cfg aws.Config, name string) (*types.Stack, error) {
	return bridge.GetStack(cfg, fmt.Sprintf(PipelineStackNameTmpl, name))
}
