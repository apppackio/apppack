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

func setRdsDeletionProtection(sess *session.Session, stack *cloudformation.Stack, protected bool) error {
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
		DeletionProtection:  &protected,
		ApplyImmediately:    aws.Bool(true),
	})
	return err
}

// confirmDeleteStack will prompt the user to confirm stack deletion and return a Stack object
func confirmDeleteStack(cfnSvc *cloudformation.CloudFormation, stackName string, friendlyName string) (*cloudformation.Stack, error) {
	startSpinner()
	stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}
	stackID := *stackOutput.Stacks[0].StackId
	Spinner.Stop()
	var confirm string
	fmt.Printf("%s\nAre you sure you want to delete %s? yes/[%s] ", aurora.Faint(stackID), friendlyName, aurora.Bold("no"))
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		return nil, fmt.Errorf("aborting due to user input")
	}
	return stackOutput.Stacks[0], nil
}

// deleteStack will delete a Clouformation Stack, optionally retrying for problematic stacks
func deleteStack(cfnSvc *cloudformation.CloudFormation, stackID string, friendlyName string, retry bool) error {
	startSpinner()
	_, err := cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
		StackName: &stackID,
	})
	if err != nil {
		return err
	}
	stack, err := waitForCloudformationStack(cfnSvc, stackID)
	if err != nil {
		return err
	}
	Spinner.Stop()
	if *stack.StackStatus != "DELETE_COMPLETE" {
		if retry {
			printWarning("deletion did not complete successfully, retrying...")
			return deleteStack(cfnSvc, stackID, friendlyName, false)
		}
		return fmt.Errorf("failed to delete %s", friendlyName)
	}
	printSuccess(fmt.Sprintf("%s destroyed", friendlyName))
	return nil
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
		sess, err := awsSession()
		checkErr(err)
		stackName := "apppack-account"
		cfnSvc := cloudformation.New(sess)
		friendlyName := "AppPack account"
		stack, err := confirmDeleteStack(cfnSvc, stackName, friendlyName)
		checkErr(err)
		err = deleteStack(cfnSvc, *stack.StackId, friendlyName, false)
		checkErr(err)
	},
}

// destroyRegionCmd represents the destroy command
var destroyRegionCmd = &cobra.Command{
	Use:                   "region",
	Short:                 "destroy AWS resources used by an AppPack region",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		sess, err := awsSession()
		checkErr(err)
		stackName := fmt.Sprintf("apppack-region-%s", *sess.Config.Region)
		ssmSvc := ssm.New(sess)
		cfnSvc := cloudformation.New(sess)
		friendlyName := fmt.Sprintf("region %s", *sess.Config.Region)
		stack, err := confirmDeleteStack(cfnSvc, stackName, friendlyName)
		checkErr(err)
		_, err = ssmSvc.DeleteParameter(&ssm.DeleteParameterInput{
			Name: aws.String("/apppack/account/dockerhub-access-token"),
		})
		if err != nil {
			printError(fmt.Sprintf("%v", err))
		}
		err = deleteStack(cfnSvc, *stack.StackId, friendlyName, false)
		checkErr(err)
	},
}

// destroyRedisCmd represents the destroy redis command
var destroyRedisCmd = &cobra.Command{
	Use:                   "redis <name>",
	Short:                 "destroy AWS resources used by an AppPack Redis instance",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		stackName := fmt.Sprintf(redisStackNameTmpl, args[0])
		sess, err := awsSession()
		checkErr(err)
		ssmSvc := ssm.New(sess)
		cfnSvc := cloudformation.New(sess)
		friendlyName := fmt.Sprintf("app %s", args[0])
		stack, err := confirmDeleteStack(cfnSvc, stackName, friendlyName)
		checkErr(err)
		err = deleteStack(cfnSvc, *stack.StackId, friendlyName, false)
		if err != nil {
			printError(fmt.Sprintf("%v", err))
		}
		_, err = ssmSvc.DeleteParameter(&ssm.DeleteParameterInput{
			Name: aws.String(fmt.Sprintf(redisAuthTokenParameterTmpl, args[0])),
		})
		checkErr(err)
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
		friendlyName := fmt.Sprintf("database %s", args[0])
		stack, err := confirmDeleteStack(cfnSvc, stackName, friendlyName)
		checkErr(err)
		err = setRdsDeletionProtection(sess, stack, false)
		if err != nil {
			printError(fmt.Sprintf("%v", err))
		}
		err = deleteStack(cfnSvc, *stack.StackId, friendlyName, false)
		checkErr(err)
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
		sess, err := awsSession()
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		friendlyName := fmt.Sprintf("cluster %s", clusterName)
		stackName := fmt.Sprintf("apppack-cluster-%s", clusterName)
		stack, err := confirmDeleteStack(cfnSvc, stackName, friendlyName)
		checkErr(err)
		// Weird circular dependency causes this https://github.com/aws/containers-roadmap/issues/631
		// Cluster depends on ASG for creation, but ASG must be deleted before the Cluster
		// retrying works around this for now
		err = deleteStack(cfnSvc, *stack.StackId, friendlyName, true)
		checkErr(err)
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
		sess, err := awsSession()
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		friendlyName := fmt.Sprintf("app %s", appName)
		stack, err := confirmDeleteStack(cfnSvc, appStackName(appName), friendlyName)
		checkErr(err)
		err = deleteStack(cfnSvc, *stack.StackId, friendlyName, false)
		checkErr(err)
	},
}

// destroyCustomDomainCmd represents the destroy custom-domain
var destroyCustomDomainCmd = &cobra.Command{
	Use:                   "custom-domain <domain>",
	Short:                 "destroy AWS resources used by the custom domain",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		primaryDomain := args[0]
		friendlyName := fmt.Sprintf("%s domain", primaryDomain)
		sess, err := awsSession()
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		stack, err := confirmDeleteStack(cfnSvc, customDomainStackName(primaryDomain), friendlyName)
		checkErr(err)
		err = deleteStack(cfnSvc, *stack.StackId, friendlyName, false)
		checkErr(err)
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to destroy resources in")

	destroyCmd.AddCommand(destroyAccountCmd)
	destroyCmd.AddCommand(destroyRegionCmd)
	destroyCmd.AddCommand(destroyClusterCmd)
	destroyCmd.AddCommand(destroyCustomDomainCmd)
	destroyCmd.AddCommand(destroyAppCmd)
	destroyCmd.AddCommand(destroyRedisCmd)
	destroyCmd.AddCommand(destroyDatabaseCmd)
}
