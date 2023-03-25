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
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

// UpgradeStackCmd updates the stack to the latest cloudformation template
func UpgradeStackCmd(sess *session.Session, stack stacks.Stack, name string) {
	ui.StartSpinner()
	checkErr(stacks.LoadStackFromCloudformation(sess, stack, &name))
	var prefix string
	if createChangeSet {
		prefix = "Creating change set for"
	} else {
		prefix = "Upgrading"
	}
	ui.Spinner.Stop()
	fmt.Print(aurora.Green(fmt.Sprintf("🔆 %s %s", prefix, stack.StackType())).String())
	if name != "" {
		fmt.Print(aurora.Green(fmt.Sprintf(" `%s`", name)).String())
	}
	fmt.Print(aurora.Green(fmt.Sprintf(" in %s", *sess.Config.Region)).String())
	if CurrentAccountRole != nil {
		fmt.Print(aurora.Green(fmt.Sprintf(" on account %s", CurrentAccountRole.GetAccountName())).String())
	}
	fmt.Println()
	ui.StartSpinner()
	if createChangeSet {
		url, err := stacks.UpdateStackChangeset(sess, stack, &name, &release)
		checkErr(err)
		ui.Spinner.Stop()
		fmt.Println("View changeset at", aurora.White(url))
	} else {
		checkErr(stacks.UpdateStack(sess, stack, &name, &release))
		ui.Spinner.Stop()
		var nameSuccessMsg string
		if name != "" {
			nameSuccessMsg = fmt.Sprintf(" for %s", name)
		}
		ui.PrintSuccess(fmt.Sprintf("updated %s stack%s", stack.StackType(), nameSuccessMsg))
	}
}

// upgradeCmd represents the upgrade command
var upgradeCmd = &cobra.Command{
	Use:                   "upgrade",
	Short:                 "upgrade AppPack stacks",
	DisableFlagsInUseLine: true,
}

// upgradeAccountCmd represents the upgrade account command
var upgradeAccountCmd = &cobra.Command{
	Use:                   "account",
	Short:                 "upgrade your AppPack account stack",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		UpgradeStackCmd(sess, &stacks.AccountStack{Parameters: &stacks.AccountStackParameters{}}, "")
	},
}

// upgradeRegionCmd represents the upgrade region command
var upgradeRegionCmd = &cobra.Command{
	Use:                   "region",
	Short:                 "upgrade your AppPack region stack",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		UpgradeStackCmd(sess, &stacks.RegionStack{Parameters: &stacks.RegionStackParameters{}}, *sess.Config.Region)
	},
}

// upgradeAppCmd represents the upgrade app command
var upgradeAppCmd = &cobra.Command{
	Use:                   "app <name>",
	Short:                 "upgrade an application AppPack stack",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		UpgradeStackCmd(sess, &stacks.AppStack{Pipeline: false, Parameters: &stacks.DefaultAppStackParameters}, args[0])
	},
}

// upgradePipelineCmd represents the upgrade command
var upgradePipelineCmd = &cobra.Command{
	Use:                   "pipeline <name>",
	Short:                 "upgrade a pipeline AppPack stack",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		UpgradeStackCmd(sess, &stacks.AppStack{Pipeline: true, Parameters: &stacks.DefaultPipelineStackParameters}, args[0])
	},
}

// upgradeClusterCmd represents the upgrade command
var upgradeClusterCmd = &cobra.Command{
	Use:                   "cluster <name>",
	Short:                 "upgrade a cluster AppPack stack",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		UpgradeStackCmd(sess, &stacks.ClusterStack{Parameters: &stacks.ClusterStackParameters{}}, args[0])
	},
}

// upgradeRedisCmd represents the upgrade command
var upgradeRedisCmd = &cobra.Command{
	Use:                   "redis <name>",
	Short:                 "upgrade a Redis AppPack stack",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		UpgradeStackCmd(sess, &stacks.RedisStack{Parameters: &stacks.DefaultRedisStackParameters}, args[0])
	},
}

// upgradeDatabaseCmd represents the upgrade command
var upgradeDatabaseCmd = &cobra.Command{
	Use:                   "database <name>",
	Short:                 "upgrade a database AppPack stack",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		UpgradeStackCmd(sess, &stacks.DatabaseStack{Parameters: &stacks.DefaultDatabaseStackParameters}, args[0])
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", "AWS account ID or alias (not needed if you are only the administrator of one account)")
	upgradeCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	upgradeCmd.PersistentFlags().BoolVar(&createChangeSet, "check", false, "check stack in Cloudformation before creating")
	upgradeCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to upgrade resources in")
	upgradeCmd.PersistentFlags().StringVar(&release, "release", "latest", "Specify a specific pre-release stack")
	upgradeCmd.PersistentFlags().MarkHidden("release")
	upgradeCmd.AddCommand(upgradeAccountCmd)
	upgradeCmd.AddCommand(upgradeClusterCmd)
	upgradeCmd.AddCommand(upgradeDatabaseCmd)
	upgradeCmd.AddCommand(upgradeRedisCmd)
	upgradeCmd.AddCommand(upgradeRegionCmd)
	upgradeCmd.AddCommand(upgradeAppCmd)
	upgradeCmd.AddCommand(upgradePipelineCmd)
}
