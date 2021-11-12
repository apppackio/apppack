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
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/logrusorgru/aurora"

	"github.com/spf13/cobra"
)

func updateChangeSetAndWait(sess *session.Session, stackInput *cloudformation.UpdateStackInput) (*cloudformation.DescribeChangeSetOutput, error) {
	cfnSvc := cloudformation.New(sess)
	changeSetName := fmt.Sprintf("update-%d", int32(time.Now().Unix()))
	_, err := cfnSvc.CreateChangeSet(&cloudformation.CreateChangeSetInput{
		ChangeSetType: aws.String("UPDATE"),
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

func updateStackAndWait(sess *session.Session, stackInput *cloudformation.UpdateStackInput) (*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(sess)
	_, err := cfnSvc.UpdateStack(stackInput)
	if err != nil {
		return nil, err
	}
	return waitForCloudformationStack(cfnSvc, *stackInput.StackName)
}

func upgradeStack(stackName, templateURL string) error {
	startSpinner()
	sess, err := adminSession(SessionDurationSeconds)
	checkErr(err)
	cfnSvc := cloudformation.New(sess)
	stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	checkErr(err)
	Spinner.Stop()
	if createChangeSet {
		fmt.Println(aurora.Faint(fmt.Sprintf("creating changeset for %s", *stackOutput.Stacks[0].StackId)))
	} else {
		fmt.Println(aurora.Faint(fmt.Sprintf("upgrading %s", *stackOutput.Stacks[0].StackId)))
	}
	var parameters []*cloudformation.Parameter
	for _, p := range stackOutput.Stacks[0].Parameters {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey:     p.ParameterKey,
			UsePreviousValue: aws.Bool(true),
		})
	}
	startSpinner()
	updateStackInput := cloudformation.UpdateStackInput{
		StackName:    &stackName,
		TemplateURL:  aws.String(getReleaseUrl(templateURL)),
		Parameters:   parameters,
		Capabilities: []*string{aws.String("CAPABILITY_IAM")},
	}
	if createChangeSet {
		changeset, err := updateChangeSetAndWait(sess, &updateStackInput)
		checkErr(err)
		Spinner.Stop()
		fmt.Println("View changeset at:", aurora.White(fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s", *sess.Config.Region, url.QueryEscape(*changeset.StackId), url.QueryEscape(*changeset.ChangeSetId))))
		printSuccess("changeset created")
	} else {
		stack, err := updateStackAndWait(sess, &updateStackInput)
		checkErr(err)
		if *stack.StackStatus != "UPDATE_COMPLETE" {
			checkErr(fmt.Errorf("stack upgrade failed: %s", *stack.StackStatus))
		}
		Spinner.Stop()
		printSuccess("stack upgraded")
	}

	return nil
}

// upgradeCmd represents the upgrade command
var upgradeCmd = &cobra.Command{
	Use:                   "upgrade",
	Short:                 "upgrade AppPack stacks",
	DisableFlagsInUseLine: true,
}

// upgradeCmd represents the upgrade command
var upgradeAppCmd = &cobra.Command{
	Use:                   "app <name>",
	Short:                 "upgrade an application AppPack stack",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		stackName := appStackName(args[0])
		err := upgradeStack(stackName, appFormationURL)
		checkErr(err)
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
		stackName := pipelineStackName(args[0])
		err := upgradeStack(stackName, appFormationURL)
		checkErr(err)
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
		stackName := fmt.Sprintf("apppack-cluster-%s", args[0])
		err := upgradeStack(stackName, clusterFormationURL)
		checkErr(err)
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
		stackName := fmt.Sprintf("apppack-redis-%s", args[0])
		err := upgradeStack(stackName, redisFormationURL)
		checkErr(err)
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
		stackName := fmt.Sprintf("apppack-database-%s", args[0])
		err := upgradeStack(stackName, databaseFormationURL)
		checkErr(err)
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", "AWS account ID or alias (not needed if you are only the administrator of one account)")
	upgradeCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	upgradeCmd.PersistentFlags().BoolVar(&createChangeSet, "check", false, "check stack in Cloudformation before creating")
	upgradeCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to upgrade resources in")
	upgradeCmd.PersistentFlags().StringVar(&release, "release", "", "Specify a specific pre-release stack")
	upgradeCmd.PersistentFlags().MarkHidden("release")
	upgradeCmd.AddCommand(upgradeClusterCmd)
	upgradeCmd.AddCommand(upgradeDatabaseCmd)
	upgradeCmd.AddCommand(upgradeRedisCmd)
	upgradeCmd.AddCommand(upgradeAppCmd)
	upgradeCmd.AddCommand(upgradePipelineCmd)
}
