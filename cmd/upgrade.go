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
	describeStacksInput := cloudformation.DescribeStacksInput{StackName: stackInput.StackName}
	err = cfnSvc.WaitUntilStackUpdateComplete(&describeStacksInput)
	if err != nil {
		return nil, err
	}
	stack, err := cfnSvc.DescribeStacks(&describeStacksInput)
	if err != nil {
		return nil, err
	}
	return stack.Stacks[0], nil
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
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess := session.Must(session.NewSession())
		cfnSvc := cloudformation.New(sess)
		appName := args[0]
		stackName := fmt.Sprintf("apppack-app-%s", appName)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		checkErr(err)
		Spinner.Stop()
		fmt.Println(aurora.Faint(fmt.Sprintf("upgrading %s", *stackOutput.Stacks[0].StackId)))
		startSpinner()
		updateStackInput := cloudformation.UpdateStackInput{
			StackName:    &stackName,
			TemplateURL:  aws.String(appFormationURL),
			Parameters:   stackOutput.Stacks[0].Parameters,
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
		}
		if createChangeSet {
			_, err = updateChangeSetAndWait(sess, &updateStackInput)
		} else {
			_, err = updateStackAndWait(sess, &updateStackInput)
		}
		checkErr(err)
		Spinner.Stop()
		printSuccess("stack upgraded")

	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.PersistentFlags().BoolVar(&createChangeSet, "check", false, "Check stack in Cloudformation before creating")
	upgradeCmd.AddCommand(upgradeAppCmd)
}
