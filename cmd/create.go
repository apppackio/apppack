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
	"errors"
	"fmt"

	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/stacks"
	"github.com/apppackio/apppack/ui"
	"github.com/apppackio/apppack/utils"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	createChangeSet   bool
	nonInteractive    bool
	region            string
	release           string
	ErrRegionNotSetup = errors.New("region isn't setup -- either change the --region or use --create-region to setup this region")
)

func CreateStackCommand(sess *session.Session, stack stacks.Stack, flags *pflag.FlagSet, name string) {
	stackName := stack.StackName(&name)
	exists, err := bridge.StackExists(sess, *stackName)
	checkErr(err)
	if *exists {
		checkErr(fmt.Errorf("stack %s already exists", *stackName))
	}
	checkErr(stack.UpdateFromFlags(flags))
	ui.Spinner.Stop()
	caser := cases.Title(language.English)
	fmt.Print(aurora.Green(fmt.Sprintf("🏗  Creating %s `%s` in %s", caser.String(stack.StackType()), name, *sess.Config.Region)).String())
	if CurrentAccountRole != nil {
		fmt.Print(aurora.Green(fmt.Sprintf(" on %s", CurrentAccountRole.GetAccountName())).String())
	}
	fmt.Println()
	if !nonInteractive {
		checkErr(stack.AskQuestions(sess))
	}
	ui.StartSpinner()
	if createChangeSet {
		url, err := stacks.CreateStackChangeset(sess, stack, &name, &release)
		checkErr(err)
		ui.Spinner.Stop()
		fmt.Println("View changeset at", aurora.White(url))
	} else {
		checkErr(stacks.CreateStack(sess, stack, &name, &release))
		ui.Spinner.Stop()
		ui.PrintSuccess(fmt.Sprintf("created %s stack %s", stack.StackType(), name))
	}
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

// appCmd represents the create app command
var appCmd = &cobra.Command{
	Use:                   "app <name>",
	Short:                 "create an AppPack application",
	Long:                  "*Requires admin permissions.*",
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		name := args[0]
		exists, err := bridge.StackExists(sess, fmt.Sprintf(stacks.PipelineStackNameTmpl, name))
		checkErr(err)
		if *exists {
			checkErr(fmt.Errorf("a pipeline named %s already exists -- app and pipeline names must be unique", name))
		}
		appParameters := stacks.DefaultAppStackParameters
		appParameters.Name = name
		CreateStackCommand(
			sess,
			&stacks.AppStack{
				Parameters: &appParameters,
				Pipeline:   false,
			},
			cmd.Flags(),
			name,
		)
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
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		name := args[0]
		exists, err := bridge.StackExists(sess, fmt.Sprintf(stacks.AppStackNameTmpl, name))
		checkErr(err)
		if *exists {
			checkErr(fmt.Errorf("an app named %s already exists -- app and pipeline names must be unique", name))
		}
		pipelineParameters := stacks.DefaultPipelineStackParameters
		pipelineParameters.Name = name
		CreateStackCommand(
			sess,
			&stacks.AppStack{
				Parameters: &pipelineParameters,
				Pipeline:   true,
			},
			cmd.Flags(),
			name,
		)
	},
}

// createClusterCmd represents the create cluster command
var createClusterCmd = &cobra.Command{
	Use:   "cluster [<name>]",
	Short: "setup resources for an AppPack Cluster",
	Long: `*Requires admin permissions.*
	Creates an AppPack Cluster. ` + "If a `<name>` is not provided, the default name, `apppack` will be used.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var name string
		if len(args) == 0 {
			name = "apppack"
		} else {
			name = args[0]
		}
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		regionExists, err := bridge.StackExists(sess, fmt.Sprintf("apppack-region-%s", *sess.Config.Region))
		checkErr(err)
		// handle region creation if the user wants
		if !*regionExists {
			ui.Spinner.Stop()
			fmt.Println(aurora.Blue(fmt.Sprintf("ℹ %s region is not initialized", *sess.Config.Region)))
			createRegion, err := cmd.Flags().GetBool("create-region")
			checkErr(err)
			if !nonInteractive && !createRegion {
				fmt.Printf("If this is your first cluster or you want to setup up a new region, type '%s' to continue.\n", aurora.White("yes"))
				fmt.Print(aurora.White(fmt.Sprintf("Create cluster in %s region? ", *sess.Config.Region)).String())
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "yes" {
					checkErr(fmt.Errorf("aborting due to user input"))
				}
			} else if !createRegion {
				checkErr(ErrRegionNotSetup)
			}
			fmt.Printf("running %s...\n", aurora.White("apppack create region"))
			createRegionCmd.Run(cmd, []string{})
			fmt.Println("")
		}
		clusterParameters := stacks.DefaultClusterStackParameters
		clusterParameters.Name = name
		CreateStackCommand(
			sess,
			&stacks.ClusterStack{
				Parameters: &clusterParameters,
			},
			cmd.Flags(),
			name,
		)
	},
}

// createCustomDomainCmd represents the createCustomDomain command
var createCustomDomainCmd = &cobra.Command{
	Use:   "custom-domain <domain-name>...",
	Args:  cobra.RangeArgs(1, 6),
	Short: "setup TLS certificate and point one or more domains to an AppPack Cluster",
	Long: `*Requires admin permissions.*

The domain(s) provided must all be a part of the same parent domain and a Route53 Hosted Zone must already be setup.`,
	Example:               "apppack create custom-domain example.com www.example.com",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		primaryDomain := args[0]
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		parameters := &stacks.CustomDomainStackParameters{}
		domainParameters := []*string{&parameters.PrimaryDomain, &parameters.AltDomain1, &parameters.AltDomain2, &parameters.AltDomain3, &parameters.AltDomain4, &parameters.AltDomain5}
		for i, domain := range args[1:] {
			*domainParameters[i] = domain
		}
		CreateStackCommand(
			sess,
			&stacks.CustomDomainStack{Parameters: parameters},
			cmd.Flags(),
			primaryDomain,
		)
	},
}

// createRedisCmd represents the create redis command
var createRedisCmd = &cobra.Command{
	Use:   "redis [<name>]",
	Short: "setup resources for an AppPack Redis instance",
	Long: `*Requires admin permissions.*
Creates an AppPack Redis instance. ` + "If a `<name>` is not provided, the default name, `apppack` will be used.",
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
		redisParameters := stacks.DefaultRedisStackParameters
		redisParameters.Name = name
		CreateStackCommand(
			sess,
			&stacks.RedisStack{
				Parameters: &redisParameters,
			},
			cmd.Flags(),
			name,
		)
	},
}

// createDatabaseCmd represents the create redis command
var createDatabaseCmd = &cobra.Command{
	Use:                   "database [<name>]",
	Short:                 "setup resources for an AppPack Database",
	Long:                  "*Requires admin permissions.*\nCreates an AppPack Database. If a `<name>` is not provided, the default name, `apppack` will be used.",
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
		dbParameters := stacks.DefaultDatabaseStackParameters
		dbParameters.Name = name
		CreateStackCommand(
			sess,
			&stacks.DatabaseStack{
				Parameters: &dbParameters,
			},
			cmd.Flags(),
			name,
		)
	},
}

// createRegionCmd represents the create command
var createRegionCmd = &cobra.Command{
	Use:                   "region",
	Short:                 "setup AppPack resources for an AWS region",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, _ []string) {
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		region := sess.Config.Region
		CreateStackCommand(
			sess,
			&stacks.RegionStack{
				Parameters: &stacks.RegionStackParameters{},
			},
			cmd.Flags(),
			*region,
		)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", utils.AccountFlagHelpText)
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
	appCmd.Flags().String("healthcheck-path", stacks.DefaultAppStackParameters.HealthCheckPath, "path which will return a 200 status code for healthchecks")
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
	pipelineCmd.Flags().String("healthcheck-path", stacks.DefaultAppStackParameters.HealthCheckPath, "path which will return a 200 status code for healthchecks")
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

	createCmd.AddCommand(createCustomDomainCmd)

	createCmd.AddCommand(createClusterCmd)
	createClusterCmd.Flags().String("domain", "", "cluster domain name")
	createClusterCmd.Flags().String("instance-class", "", "ec2 cluster autoscaling instance class -- see https://aws.amazon.com/ec2/pricing/on-demand/")
	createClusterCmd.Flags().MarkHidden("instance-class")
	createClusterCmd.Flags().String("cidr", stacks.DefaultClusterStackParameters.Cidr, "network CIDR for VPC")
	createClusterCmd.Flags().Bool("create-region", false, "also create the region stack if it does not exist")

	createCmd.AddCommand(createRedisCmd)
	createRedisCmd.Flags().String("cluster", "apppack", "cluster name")
	createRedisCmd.Flags().String("instance-class", stacks.DefaultRedisStackParameters.InstanceClass, "instance class -- see https://aws.amazon.com/elasticache/pricing/#On-Demand_Nodes")
	createRedisCmd.Flags().Bool("multi-az", stacks.DefaultRedisStackParameters.MultiAZ, "enable multi-AZ -- see https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html")

	createCmd.AddCommand(createDatabaseCmd)
	createDatabaseCmd.Flags().String("cluster", "apppack", "cluster name")
	createDatabaseCmd.Flags().StringP("instance-class", "i", stacks.DefaultDatabaseStackParameters.InstanceClass, "instance class -- see https://aws.amazon.com/rds/postgresql/pricing/?pg=pr&loc=3")
	createDatabaseCmd.Flags().StringP("engine", "e", stacks.DefaultDatabaseStackParameters.Engine, "engine -- one of mysql, postgres, aurora-mysql, aurora-postgresql")
	createDatabaseCmd.Flags().Bool("multi-az", stacks.DefaultDatabaseStackParameters.MultiAZ, "enable multi-AZ -- see https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZ.html")
	createDatabaseCmd.Flags().Int("allocated-storage", stacks.DefaultDatabaseStackParameters.AllocatedStorage, "initial storage allocated in GB (does not apply to Aurora engines)")
	createDatabaseCmd.Flags().Int("max-allocated-storage", stacks.DefaultDatabaseStackParameters.MaxAllocatedStorage, "maximum storage allocated on-demand in GB (does not apply to Aurora engines)")

	createCmd.AddCommand(createRegionCmd)
}
