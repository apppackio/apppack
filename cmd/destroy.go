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
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

type accountDetails struct {
	StackID string `json:"stack_id"`
}

// destroyCmd represents the destroy command
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy AWS resources used by AppPack",
	Long:  `Destroy AWS resources used by AppPack`,
}

// destroyAccountCmd represents the destroy command
var destroyAccountCmd = &cobra.Command{
	Use:   "account",
	Short: "Destroy AWS resources used by your AppPack account",
	Long:  `Destroy AWS resources used by your AppPack account`,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess := session.Must(session.NewSession())
		ssmSvc := ssm.New(sess)
		paramOutput, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
			Name: aws.String("/paaws/account"),
		})
		checkErr(err)
		var account accountDetails
		err = json.Unmarshal([]byte(*paramOutput.Parameter.Value), &account)
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		_, err = cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &account.StackID,
		})
		checkErr(err)
		Spinner.Stop()
		var confirm string
		fmt.Printf("Are you sure you want to delete your AppPack Account Stack\n%s? yes/[%s]\n", aurora.Faint(account.StackID), aurora.Bold("no"))
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			checkErr(fmt.Errorf("aborting due to user input"))
		}
		startSpinner()
		_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
			StackName: &account.StackID,
		})
		checkErr(err)
		err = cfnSvc.WaitUntilStackDeleteComplete(&cloudformation.DescribeStacksInput{
			StackName: &account.StackID,
		})
		_, err1 := ssmSvc.DeleteParameter(&ssm.DeleteParameterInput{
			Name: aws.String("/paaws/account/dockerhub-access-token"),
		})
		Spinner.Stop()
		checkErr(err)
		checkErr(err1)
		printSuccess("AppPack account deleted")
	},
}

// destroyClusterCmd represents the destroy command
var destroyClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Destroy AWS resources used by the AppPack cluster",
	Long:  `Destroy AWS resources used by the AppPack cluster`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		clusterName := args[0]
		startSpinner()
		sess := session.Must(session.NewSession())
		stackName := fmt.Sprintf("apppack-cluster-%s", clusterName)
		cfnSvc := cloudformation.New(sess)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		stackID := *stackOutput.Stacks[0].StackId
		checkErr(err)
		Spinner.Stop()
		var confirm string
		fmt.Printf("Are you sure you want to delete your AppPack cluster %s\n%s? yes/[%s]\n", clusterName, aurora.Faint(stackID), aurora.Bold("no"))
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			checkErr(fmt.Errorf("aborting due to user input"))
		}
		startSpinner()
		_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
			StackName: &stackID,
		})
		checkErr(err)
		err = cfnSvc.WaitUntilStackDeleteComplete(&cloudformation.DescribeStacksInput{
			StackName: &stackID,
		})
		// Weird circular dependency causes this https://github.com/aws/containers-roadmap/issues/631
		// Cluster depends on ASG for creation, but ASG must be deleted before the Cluster
		// retrying works around this for now
		if err != nil {
			Spinner.Stop()
			printWarning("cluster deletion did not complete successfully, retrying...")
			startSpinner()
			_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
				StackName: &stackID,
			})
			checkErr(err)
			err = cfnSvc.WaitUntilStackDeleteComplete(&cloudformation.DescribeStacksInput{
				StackName: &stackID,
			})
			checkErr(err)
		}
		Spinner.Stop()
		printSuccess(fmt.Sprintf("AppPack cluster %s destroyed", clusterName))
	},
}

// destroyAppCmd represents the destroy command
var destroyAppCmd = &cobra.Command{
	Use:   "app",
	Short: "Destroy AWS resources used by the AppPack app",
	Long:  `Destroy AWS resources used by the AppPack app`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		appName := args[0]
		startSpinner()
		sess := session.Must(session.NewSession())
		stackName := fmt.Sprintf("apppack-app-%s", appName)
		cfnSvc := cloudformation.New(sess)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		checkErr(err)
		stackID := *stackOutput.Stacks[0].StackId
		Spinner.Stop()
		var confirm string
		fmt.Printf("Are you sure you want to delete your AppPack app %s\n%s? yes/[%s]\n", appName, aurora.Faint(stackID), aurora.Bold("no"))
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			checkErr(fmt.Errorf("aborting due to user input"))
		}
		startSpinner()
		_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
			StackName: &stackID,
		})
		checkErr(err)
		err = cfnSvc.WaitUntilStackDeleteComplete(&cloudformation.DescribeStacksInput{
			StackName: &stackID,
		})
		Spinner.Stop()
		checkErr(err)
		printSuccess(fmt.Sprintf("AppPack app %s destroyed", appName))
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)

	destroyCmd.AddCommand(destroyAccountCmd)
	destroyCmd.AddCommand(destroyClusterCmd)
	destroyCmd.AddCommand(destroyAppCmd)
}
