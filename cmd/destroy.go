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
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

type accountDetails struct {
	StackID string `json:"stack_id"`
}

func disableDBDeletionProtection(sess *session.Session, stack *cloudformation.Stack) error {
	rdsSvc := rds.New(sess)
	clusterID := ""
	for _, output := range stack.Outputs {
		if *output.OutputKey == "DBClusterId" {
			clusterID = *output.OutputValue
			break
		}
	}
	if clusterID == "" {
		return fmt.Errorf("unable to retrieve DBClusterID from %s", *stack.StackId)
	}
	_, err := rdsSvc.ModifyDBCluster(&rds.ModifyDBClusterInput{
		DBClusterIdentifier: &clusterID,
		DeletionProtection:  aws.Bool(false),
		ApplyImmediately:    aws.Bool(true),
	})
	return err
}

// destroyCmd represents the destroy command
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "destroy AWS resources used by AppPack",
	Long:  "All `destroy` subcommands require AWS credentials.",
}

// destroyAccountCmd represents the destroy command
var destroyAccountCmd = &cobra.Command{
	Use:                   "account",
	Short:                 "destroy AWS resources used by your AppPack account",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess, err := awsSession()
		checkErr(err)
		ssmSvc := ssm.New(sess)
		paramOutput, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
			Name: aws.String("/apppack/account"),
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
		stack, err := waitForCloudformationStack(cfnSvc, account.StackID)
		Spinner.Stop()
		checkErr(err)
		if *stack.StackStatus != "DELETE_COMPLETE" {
			checkErr(fmt.Errorf("Account deletion failed. current state: %s", *stack.StackStatus))
		}
		printSuccess("AppPack account deleted")
	},
}

// destroyRegionCmd represents the destroy command
var destroyRegionCmd = &cobra.Command{
	Use:                   "region",
	Short:                 "destroy AWS resources used by an AppPack region",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess, err := awsSession()
		checkErr(err)
		ssmSvc := ssm.New(sess)
		cfnSvc := cloudformation.New(sess)
		stackName := fmt.Sprintf("apppack-region-%s", *sess.Config.Region)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		checkErr(err)
		stack := stackOutput.Stacks[0]
		Spinner.Stop()
		var confirm string
		fmt.Printf("Are you sure you want to delete your AppPack Region Stack\n%s? yes/[%s]\n", aurora.Faint(*stack.StackId), aurora.Bold("no"))
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			checkErr(fmt.Errorf("aborting due to user input"))
		}
		startSpinner()
		_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
			StackName: stack.StackId,
		})
		checkErr(err)
		stack, err = waitForCloudformationStack(cfnSvc, *stack.StackId)
		_, err1 := ssmSvc.DeleteParameter(&ssm.DeleteParameterInput{
			Name: aws.String("/apppack/account/dockerhub-access-token"),
		})
		Spinner.Stop()
		checkErr(err)
		checkErr(err1)
		if *stack.StackStatus != "DELETE_COMPLETE" {
			checkErr(fmt.Errorf("Region deletion failed. current state: %s", *stack.StackStatus))
		}
		printSuccess("AppPack region deleted")
	},
}

// destroyRedisCmd represents the destroy redis command
var destroyRedisCmd = &cobra.Command{
	Use:                   "redis <name>",
	Short:                 "destroy AWS resources used by an AppPack Redis instance",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess, err := awsSession()
		checkErr(err)
		ssmSvc := ssm.New(sess)
		cfnSvc := cloudformation.New(sess)
		stackName := fmt.Sprintf(redisStackNameTmpl, args[0])
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		checkErr(err)
		stack := stackOutput.Stacks[0]
		Spinner.Stop()
		var confirm string
		fmt.Printf("Are you sure you want to delete your AppPack Redis Stack\n%s? yes/[%s]\n", aurora.Faint(*stack.StackId), aurora.Bold("no"))
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			checkErr(fmt.Errorf("aborting due to user input"))
		}
		startSpinner()
		_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
			StackName: stack.StackId,
		})
		checkErr(err)
		stack, err = waitForCloudformationStack(cfnSvc, *stack.StackId)
		_, err1 := ssmSvc.DeleteParameter(&ssm.DeleteParameterInput{
			Name: aws.String(fmt.Sprintf(redisAuthTokenParameterTmpl, args[0])),
		})
		Spinner.Stop()
		checkErr(err)
		checkErr(err1)
		if *stack.StackStatus != "DELETE_COMPLETE" {
			checkErr(fmt.Errorf("Redis deletion failed. current state: %s", *stack.StackStatus))
		}
		printSuccess("AppPack Redis instance deleted")
	},
}

// destroyDatabaseCmd represents the destroy database command
var destroyDatabaseCmd = &cobra.Command{
	Use:                   "database <name>",
	Short:                 "destroy AWS resources used by an AppPack Database",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess, err := awsSession()
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		stackName := fmt.Sprintf(databaseStackNameTmpl, args[0])
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		checkErr(err)
		stack := stackOutput.Stacks[0]
		Spinner.Stop()
		var confirm string
		fmt.Printf("Are you sure you want to delete your AppPack Database Stack\n%s? yes/[%s]\n", aurora.Faint(*stack.StackId), aurora.Bold("no"))
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			checkErr(fmt.Errorf("aborting due to user input"))
		}
		startSpinner()
		err = disableDBDeletionProtection(sess, stack)
		checkErr(err)
		_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
			StackName: stack.StackId,
		})
		checkErr(err)
		stack, err = waitForCloudformationStack(cfnSvc, *stack.StackId)
		checkErr(err)
		if *stack.StackStatus != "DELETE_COMPLETE" {
			checkErr(fmt.Errorf("database deletion failed. current state: %s", *stack.StackStatus))
		}
		Spinner.Stop()
		printSuccess("AppPack Database deleted")
	},
}

// destroyClusterCmd represents the destroy command
var destroyClusterCmd = &cobra.Command{
	Use:                   "cluster <name>",
	Short:                 "destroy AWS resources used by the AppPack Cluster",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		clusterName := args[0]
		startSpinner()
		sess, err := awsSession()
		checkErr(err)
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
		stack, err := waitForCloudformationStack(cfnSvc, stackID)
		// Weird circular dependency causes this https://github.com/aws/containers-roadmap/issues/631
		// Cluster depends on ASG for creation, but ASG must be deleted before the Cluster
		// retrying works around this for now
		if *stack.StackStatus != "DELETE_COMPLETE" {
			Spinner.Stop()
			printWarning("cluster deletion did not complete successfully, retrying...")
			startSpinner()
			_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
				StackName: &stackID,
			})
			checkErr(err)
			stack, err := waitForCloudformationStack(cfnSvc, stackID)
			checkErr(err)
			if *stack.StackStatus != "DELETE_COMPLETE" {
				checkErr(fmt.Errorf("cluster deletion failed. current state: %s", *stack.StackStatus))
			}
		}
		Spinner.Stop()
		printSuccess(fmt.Sprintf("AppPack cluster %s destroyed", clusterName))
	},
}

// destroyAppCmd represents the destroy command
var destroyAppCmd = &cobra.Command{
	Use:                   "app <name>",
	Short:                 "destroy AWS resources used by the AppPack app",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		appName := args[0]
		startSpinner()
		sess, err := awsSession()
		checkErr(err)
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
		stack, err := waitForCloudformationStack(cfnSvc, stackID)
		checkErr(err)
		Spinner.Stop()
		if *stack.StackStatus != "DELETE_COMPLETE" {
			checkErr(fmt.Errorf("failed to delete app %s", appName))
		}
		printSuccess(fmt.Sprintf("AppPack app %s destroyed", appName))
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to destroy resources in")

	destroyCmd.AddCommand(destroyAccountCmd)
	destroyCmd.AddCommand(destroyRegionCmd)
	destroyCmd.AddCommand(destroyClusterCmd)
	destroyCmd.AddCommand(destroyAppCmd)
	destroyCmd.AddCommand(destroyRedisCmd)
	destroyCmd.AddCommand(destroyDatabaseCmd)
}
