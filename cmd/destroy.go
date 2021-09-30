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
	"github.com/spf13/cobra"
)

func getStackOutput(stack *cloudformation.Stack, name string) (*string, error) {
	for _, output := range stack.Outputs {
		if *output.OutputKey == name {
			return output.OutputValue, nil
		}
	}
	return nil, fmt.Errorf("stack %s does not have an output named %s", *stack.StackName, name)
}

func getStackParameter(stack *cloudformation.Stack, name string) (*string, error) {
	for _, parameter := range stack.Parameters {
		if *parameter.ParameterKey == name {
			return parameter.ParameterValue, nil
		}
	}
	return nil, fmt.Errorf("stack %s does not have a parameter named %s", *stack.StackName, name)
}

func setRdsDeletionProtection(sess *session.Session, stack *cloudformation.Stack, protected bool) error {
	rdsSvc := rds.New(sess)
	DBID, err := getStackOutput(stack, "DBId")
	if err != nil {
		return err
	}
	DBType, err := getStackOutput(stack, "DBType")
	if err != nil {
		return err
	}
	if *DBType == "instance" {
		_, err := rdsSvc.ModifyDBInstance(&rds.ModifyDBInstanceInput{
			DBInstanceIdentifier: DBID,
			DeletionProtection:   &protected,
			ApplyImmediately:     aws.Bool(true),
		})
		return err
	}
	if *DBType == "cluster" {
		_, err := rdsSvc.ModifyDBCluster(&rds.ModifyDBClusterInput{
			DBClusterIdentifier: DBID,
			DeletionProtection:  &protected,
			ApplyImmediately:    aws.Bool(true),
		})
		return err
	}
	return fmt.Errorf("unexpected DB type %s", *DBType)
}

// confirmDeleteStack will prompt the user to confirm stack deletion and return a Stack object
func confirmDeleteStack(cfnSvc *cloudformation.CloudFormation, stackName, friendlyName string) (*cloudformation.Stack, error) {
	startSpinner()
	stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}
	stack := *stackOutput.Stacks[0]
	Spinner.Stop()
	confirmAction(fmt.Sprintf("This will permanently destroy all resources in the %s stack.", *stack.StackName), *stack.StackName)
	startSpinner()
	return &stack, nil
}

// deleteStack will delete a Clouformation Stack, optionally retrying for problematic stacks
func deleteStack(cfnSvc *cloudformation.CloudFormation, stackID, friendlyName string, retry bool) error {
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
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		stackName := fmt.Sprintf(redisStackNameTmpl, args[0])
		sess, err := awsSession()
		checkErr(err)
		ssmSvc := ssm.New(sess)
		cfnSvc := cloudformation.New(sess)
		friendlyName := fmt.Sprintf("Redis stack %s", args[0])
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
	Args:                  cobra.ExactArgs(1),
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

// destroyAppCmd represents the destroy app command
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

// destroyPipelineCmd represents the destroy pipeline command
var destroyPipelineCmd = &cobra.Command{
	Use:                   "pipeline <name>",
	Short:                 "destroy AWS resources used by the AppPack pipeline",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pipelineName := args[0]
		sess, err := awsSession()
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		friendlyName := fmt.Sprintf("pipeline %s", pipelineName)
		stack, err := confirmDeleteStack(cfnSvc, pipelineStackName(pipelineName), friendlyName)
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
	destroyCmd.AddCommand(destroyPipelineCmd)
	destroyCmd.AddCommand(destroyRedisCmd)
	destroyCmd.AddCommand(destroyDatabaseCmd)
}
