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

// import (
// 	"fmt"
// 	"strings"

// 	"github.com/apppackio/apppack/stacks"
// 	"github.com/apppackio/apppack/ui"
// 	"github.com/aws/aws-sdk-go/aws/session"
// 	"github.com/logrusorgru/aurora"
// 	"github.com/spf13/cobra"
// 	"github.com/spf13/pflag"
// )

// // modifyCmd represents the create command
// var modifyCmd = &cobra.Command{
// 	Use:   "modify",
// 	Short: "modify AppPack resources in your AWS account",
// 	Long: `Use subcommands to modify AppPack resources in your account.

// These require administrator access.
// `,
// 	DisableFlagsInUseLine: true,
// }

// func ModifyStackCommand(sess *session.Session, stack stacks.Stack, flags *pflag.FlagSet, name string) {
// 	ui.StartSpinner()
// 	checkErr(stacks.LoadStackFromCloudformation(sess, stack, &name))
// 	checkErr(stack.UpdateFromFlags(flags))
// 	var prefix string
// 	if createChangeSet {
// 		prefix = "Creating changeset for"
// 	} else {
// 		prefix = "Updating"
// 	}
// 	ui.Spinner.Stop()
// 	fmt.Print(aurora.Green(fmt.Sprintf("✏️  %s %s `%s` in %s", prefix, strings.Title(stack.StackType()), name, *sess.Config.Region)).String())
// 	if CurrentAccountRole != nil {
// 		fmt.Print(aurora.Green(fmt.Sprintf(" on account %s", CurrentAccountRole.GetAccountName())).String())
// 	}
// 	fmt.Println()
// 	if !nonInteractive {
// 		checkErr(stack.AskQuestions(sess))
// 	}
// 	ui.StartSpinner()
// 	if createChangeSet {
// 		url, err := stacks.ModifyStackChangeset(sess, stack, &name)
// 		checkErr(err)
// 		ui.Spinner.Stop()
// 		fmt.Println("View changeset at", aurora.White(url))
// 	} else {
// 		checkErr(stacks.ModifyStack(sess, stack, &name))
// 		ui.Spinner.Stop()
// 		ui.PrintSuccess(fmt.Sprintf("modified %s stack for %s", stack.StackType(), name))
// 	}
// }

// // modifyAppCmd represents the modify app command
// var modifyAppCmd = &cobra.Command{
// 	Use:     "app",
// 	Short:   "modify the settings for an app",
// 	Args:    cobra.ExactArgs(1),
// 	Example: "apppack modify app <appname>",
// 	Long: `Modify the settings for an app after creation.

// Requires administrator privileges.`,
// 	DisableFlagsInUseLine: true,
// 	Run: func(cmd *cobra.Command, args []string) {
// 		ui.StartSpinner()
// 		sess, err := adminSession(SessionDurationSeconds)
// 		checkErr(err)
// 		ModifyStackCommand(sess, &stacks.AppStack{Pipeline: false, Parameters: &stacks.DefaultAppStackParameters}, cmd.Flags(), args[0])
// 	},
// }

// func init() {
// 	rootCmd.AddCommand(modifyCmd)
// 	modifyCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", "AWS account ID or alias (not needed if you are only the administrator of one account)")
// 	modifyCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
// 	modifyCmd.PersistentFlags().BoolVar(&createChangeSet, "check", false, "check stack in Cloudformation before creating")
// 	modifyCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "do not prompt for user input")
// 	modifyCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to create resources in")

// 	modifyCmd.AddCommand(modifyAppCmd)
// 	modifyAppCmd.Flags().StringP("repository", "r", "", "repository URL, e.g. https://github.com/apppackio/apppack-demo-python.git")
// 	modifyAppCmd.Flags().StringP("branch", "b", "", "branch to setup for continuous deployment")
// 	modifyAppCmd.Flags().StringP("domain", "d", "", "custom domain to route to app (optional)")
// 	modifyAppCmd.Flags().String("healthcheck-path", "/", "path which will return a 200 status code for healthchecks")
// 	modifyAppCmd.Flags().Bool("addon-private-s3", false, "setup private S3 bucket add-on")
// 	modifyAppCmd.Flags().Bool("addon-public-s3", false, "setup public S3 bucket add-on")
// 	modifyAppCmd.Flags().String("addon-database-name", "", "database instance to install add-on")
// 	modifyAppCmd.Flags().String("addon-redis-name", "", "Redis instance to install add-on")
// 	modifyAppCmd.Flags().Bool("addon-sqs", false, "setup SQS Queue add-on")
// 	modifyAppCmd.Flags().String("addon-ses-domain", "", "domain approved for sending via SES add-on. Use '*' for all domains.")
// 	modifyAppCmd.Flags().StringSliceP("users", "u", []string{}, "email addresses for users who can manage the app (comma separated)")
// 	modifyAppCmd.Flags().Bool("disable-build-webhook", false, "disable creation of a webhook on the repo to automatically trigger builds on push")

// 	// pipelineCmd.Flags().SortFlags = false
// 	// pipelineCmd.Flags().StringP("repository", "r", "", "repository URL, e.g. https://github.com/apppackio/apppack-demo-python.git")
// 	// pipelineCmd.Flags().String("healthcheck-path", "/", "path which will return a 200 status code for healthchecks")
// 	// pipelineCmd.Flags().Bool("addon-private-s3", false, "setup private S3 bucket add-on")
// 	// pipelineCmd.Flags().Bool("addon-public-s3", false, "setup public S3 bucket add-on")
// 	// pipelineCmd.Flags().Bool("addon-database", false, "setup database add-on")
// 	// pipelineCmd.Flags().String("addon-database-name", "", "database instance to install add-on")
// 	// pipelineCmd.Flags().Bool("addon-redis", false, "setup Redis add-on")
// 	// pipelineCmd.Flags().String("addon-redis-name", "", "Redis instance to install add-on")
// 	// pipelineCmd.Flags().Bool("addon-sqs", false, "setup SQS Queue add-on")
// 	// pipelineCmd.Flags().Bool("addon-ses", false, "setup SES (Email) add-on (requires manual approval of domain at SES)")
// 	// pipelineCmd.Flags().String("addon-ses-domain", "*", "domain approved for sending via SES add-on. Use '*' for all domains.")
// 	// pipelineCmd.Flags().StringSliceP("users", "u", []string{}, "email addresses for users who can manage the app (comma separated)")
// }
