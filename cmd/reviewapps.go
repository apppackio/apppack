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
	"strings"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/stacks"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

func accountFlagIgnoredWarning() {
	if AccountIDorAlias != "" {
		fmt.Println(aurora.Yellow("Warning: The 'account' flag is ignored for reviewapps and all its subcommands. Reviewapps depend on access to the pipeline rather than access to the account."))
	}
}

// reviewappsCmd represents the reviewapps command
var reviewappsCmd = &cobra.Command{
	Use:                   "reviewapps <pipeline>",
	Short:                 "list deployed review apps",
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		accountFlagIgnoredWarning()
		a, err := app.Init(args[0], UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		reviewApps, err := a.GetReviewApps()
		checkErr(err)
		ui.Spinner.Stop()
		ui.PrintHeaderln(fmt.Sprintf("%s review apps", a.Name))
		for _, r := range reviewApps {
			if r.Status == "created" {
				prNumber := strings.Split(r.PullRequest, "/")[1]
				fmt.Printf("%s %s\n", aurora.White(fmt.Sprintf("#%s %s", prNumber, r.Title)), aurora.Faint(r.Branch))
				url, err := a.URL(&prNumber)
				checkErr(err)
				indent := len(prNumber) + 1
				fmt.Printf("%s %s\n\n", strings.Repeat(" ", indent), aurora.Underline(aurora.Blue(*url)))
			}
		}
	},
}

// reviewappsCreateCmd represents the create command
var reviewappsCreateCmd = &cobra.Command{
	Use:                   "create <pipeline:pr-number>",
	Short:                 "create a review app",
	Long:                  `Creates a review app from a pull request on the pipeline repository and triggers the iniital build`,
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		accountFlagIgnoredWarning()
		name := args[0]
		if len(strings.Split(name, ":")) != 2 {
			checkErr(fmt.Errorf("invalid review app name -- must be in format <pipeline>:<pr-number>"))
		}
		a, err := app.Init(args[0], UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		if !a.IsReviewApp() { // TODO: validate
			checkErr(fmt.Errorf("no pull request number set"))
		}

		stack := stacks.ReviewAppStack{
			Parameters: &stacks.ReviewAppStackParameters{},
		}
		stackName := stack.StackName(&args[0])
		exists, err := bridge.StackExists(a.Session, *stackName)
		checkErr(err)
		if *exists {
			checkErr(fmt.Errorf("stack %s already exists", *stackName))
		}
		ui.Spinner.Stop()
		fmt.Println(aurora.Green(fmt.Sprintf("🏗  Creating Review App `%s`", name)).String())
		ui.StartSpinner()
		params, err := stacks.ExportParameters(stack.GetParameters(), a.Session, &name)
		checkErr(err)
		cfnRole, err := stack.CfnRole(a.Session)
		checkErr(err)
		cfnStack, err := stacks.CreateStackAndWait(a.Session, &cloudformation.CreateStackInput{
			StackName:    stack.StackName(&name),
			Parameters:   params,
			Capabilities: stack.Capabilities(),
			RoleARN:      cfnRole,
			Tags:         stack.Tags(&name),
			TemplateURL:  stack.TemplateURL(&release),
		})
		checkErr(err)
		if *cfnStack.StackStatus != "CREATE_COMPLETE" {
			checkErr(fmt.Errorf("stack creation failed: %s", *cfnStack.StackStatus))
		}
		ui.Spinner.Stop()
		ui.PrintSuccess("review app stack created")
		ui.Spinner.Suffix = " triggering initial build..."
		ui.StartSpinner()
		build, err := a.StartBuild(true)
		checkErr(err)
		buildStatus, err := pollBuildStatus(a, int(*build.BuildNumber), 10)
		checkErr(err)
		err = watchBuild(a, buildStatus)
		checkErr(err)
		ui.Spinner.Stop()
	},
}

// reviewappsDestroyCmd represents the destroy command
var reviewappsDestroyCmd = &cobra.Command{
	Use:                   "destroy <pipeline:pr-number>",
	Short:                 "destroys the review app",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		accountFlagIgnoredWarning()
		a, err := app.Init(args[0], UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		if !a.IsReviewApp() { // TODO: validate
			checkErr(fmt.Errorf("no pull request number set"))
		}
		DestroyStackCmd(a.Session, &stacks.ReviewAppStack{Parameters: &stacks.ReviewAppStackParameters{}}, args[0])
	},
}

func init() {
	rootCmd.AddCommand(reviewappsCmd)
	reviewappsCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	reviewappsCmd.AddCommand(reviewappsCreateCmd)
	reviewappsCreateCmd.Flags().StringVar(&release, "release", "", "Specify a specific pre-release stack")
	upgradeCmd.PersistentFlags().MarkHidden("release")
	reviewappsCmd.AddCommand(reviewappsDestroyCmd)

	reviewappsCmd.PersistentFlags().StringVarP(
		&AccountIDorAlias,
		"account",
		"c",
		"",
		"AWS account ID or alias (not needed if you are only the administrator of one account)",
	)
}
