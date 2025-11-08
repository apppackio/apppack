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

	"github.com/apppackio/apppack/stacks"
	"github.com/apppackio/apppack/ui"
	"github.com/apppackio/apppack/utils"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// DestroyStackCmd destroys the given stack after user confirmation
func DestroyStackCmd(sess *session.Session, stack stacks.Stack, name string) {
	ui.StartSpinner()

	if err := stacks.LoadStackFromCloudformation(sess, stack, &name); err != nil {
		// if the stack doesn't exist, try to do the post-cleanup
		// to make sure we don't leave orphaned resources
		if err1 := stack.PostDelete(sess, &name); err1 != nil {
			logrus.WithFields(logrus.Fields{"err": err1}).Warning("post-delete failed")
		}

		checkErr(err)
	}

	stackName := stack.GetStack().StackName

	ui.Spinner.Stop()

	caser := cases.Title(language.English)
	confirmAction(fmt.Sprintf("This will permanently destroy all resources in the `%s` %s stack.", name, caser.String(stack.StackType())), *stackName)
	ui.StartSpinner()
	// retry deletion once on failure for transient errors
	retry := true

	var destroy func() error
	destroy = func() error {
		stack, err := stacks.DeleteStackAndWait(sess, stack)
		if err != nil {
			return err
		}

		ui.Spinner.Stop()

		if *stack.StackStatus != stacks.DeleteComplete {
			if retry {
				ui.PrintWarning("deletion did not complete successfully, retrying...")

				retry = false

				destroy()
			}

			return fmt.Errorf("failed to delete %s", *stack.StackName)
		}

		return nil
	}
	checkErr(destroy())
	ui.Spinner.Stop()
	ui.PrintSuccess(fmt.Sprintf("destroyed %s stack %s", stack.StackType(), name))
}

// destroyCmd represents the destroy command
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "destroy AWS resources used by AppPack",
	Long:  "All `destroy` subcommands admin permissions.",
}

// destroyAccountCmd represents the destroy command
var destroyAccountCmd = &cobra.Command{
	Use:                   "account",
	Short:                 "destroy AWS resources used by your AppPack account",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		DestroyStackCmd(sess, &stacks.AccountStack{Parameters: &stacks.AccountStackParameters{}}, "")
	},
}

// destroyRegionCmd represents the destroy command
var destroyRegionCmd = &cobra.Command{
	Use:                   "region",
	Short:                 "destroy AWS resources used by an AppPack region",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		DestroyStackCmd(sess, &stacks.RegionStack{Parameters: &stacks.RegionStackParameters{}}, *sess.Config.Region)
	},
}

// destroyRedisCmd represents the destroy redis command
var destroyRedisCmd = &cobra.Command{
	Use:                   "redis <name>",
	Short:                 "destroy AWS resources used by an AppPack Redis instance",
	Long:                  "*Requires admin permissions.*",
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		DestroyStackCmd(sess, &stacks.RedisStack{Parameters: &stacks.RedisStackParameters{}}, args[0])
	},
}

// destroyDatabaseCmd represents the destroy database command
var destroyDatabaseCmd = &cobra.Command{
	Use:                   "database <name>",
	Short:                 "destroy AWS resources used by an AppPack Database",
	Long:                  "*Requires admin permissions.*",
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		DestroyStackCmd(sess, &stacks.DatabaseStack{Parameters: &stacks.DefaultDatabaseStackParameters}, args[0])
	},
}

// destroyClusterCmd represents the destroy command
var destroyClusterCmd = &cobra.Command{
	Use:                   "cluster <name>",
	Short:                 "destroy AWS resources used by the AppPack Cluster",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		DestroyStackCmd(sess, &stacks.ClusterStack{Parameters: &stacks.ClusterStackParameters{}}, args[0])
	},
}

// destroyAppCmd represents the destroy app command
var destroyAppCmd = &cobra.Command{
	Use:                   "app <name>",
	Short:                 "destroy AWS resources used by the AppPack app",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		DestroyStackCmd(sess, &stacks.AppStack{Pipeline: false, Parameters: &stacks.AppStackParameters{}}, args[0])
	},
}

// destroyPipelineCmd represents the destroy pipeline command
var destroyPipelineCmd = &cobra.Command{
	Use:                   "pipeline <name>",
	Short:                 "destroy AWS resources used by the AppPack pipeline",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		DestroyStackCmd(sess, &stacks.AppStack{Pipeline: true, Parameters: &stacks.AppStackParameters{}}, args[0])
	},
}

// destroyCustomDomainCmd represents the destroy custom-domain
var destroyCustomDomainCmd = &cobra.Command{
	Use:                   "custom-domain <domain>",
	Short:                 "destroy AWS resources used by the custom domain",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		DestroyStackCmd(sess, &stacks.CustomDomainStack{Parameters: &stacks.CustomDomainStackParameters{}}, args[0])
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", utils.AccountFlagHelpText)
	destroyCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	destroyCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to destroy resources in")

	destroyCmd.AddCommand(destroyAccountCmd)
	destroyCmd.AddCommand(destroyRegionCmd)
	destroyCmd.AddCommand(destroyClusterCmd)
	destroyCmd.AddCommand(destroyCustomDomainCmd)
	destroyCmd.AddCommand(destroyAppCmd)
	destroyCmd.AddCommand(destroyPipelineCmd)
	destroyCmd.AddCommand(destroyRedisCmd)
	destroyCmd.AddCommand(destroyDatabaseCmd)
}
