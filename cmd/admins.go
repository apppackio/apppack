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
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/spf13/cobra"
)

// accessCmd represents the access command
var adminsCmd = &cobra.Command{
	Use:                   "admins",
	Short:                 "list the administrators for an accouunt",
	Long:                  "*Requires admin permissions.*",
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: aws.String(accountStackName),
		})
		checkErr(err)
		stack := stackOutput.Stacks[0]
		usersCSV, err := parameterValue(stack, "Administrators")
		checkErr(err)
		users := splitAndTrimCSV(usersCSV)
		sort.Strings(users)
		Spinner.Stop()
		for _, u := range users {
			fmt.Println(u)
		}
	},
}

// adminsAddCmd represents the admins add command
var adminsAddCmd = &cobra.Command{
	Use:                   "add <email>",
	Short:                 "add an administrator to the account",
	Long:                  "*Requires admin permissions.*\nUpdates the account Cloudformation stack to add administrator access for the user.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		email := args[0]
		if !validateEmail(email) {
			checkErr(fmt.Errorf("%s does not appear to be a valid email address", email))
		}
		startSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: aws.String(accountStackName),
		})
		checkErr(err)
		stack := stackOutput.Stacks[0]
		usersCSV, err := parameterValue(stack, "Administrators")
		checkErr(err)
		usersCSV = aws.String(strings.Join([]string{*usersCSV, email}, ","))
		checkErr(replaceParameter(stack, "Administrators", usersCSV))
		stack, err = updateStackAndWait(sess, &cloudformation.UpdateStackInput{
			StackName:           stack.StackName,
			Parameters:          stack.Parameters,
			UsePreviousTemplate: aws.Bool(true),
			Capabilities:        []*string{aws.String("CAPABILITY_IAM")},
		})
		checkErr(err)
		if *stack.StackStatus != cloudformation.StackStatusUpdateComplete {
			checkErr(fmt.Errorf("stack did not update successfully -- %s", *stack.StackStatus))
		}
		Spinner.Stop()
		printSuccess(fmt.Sprintf("%s added as an administrator", email))
	},
}

// adminsRemoveCmd represents the admins remove command
var adminsRemoveCmd = &cobra.Command{
	Use:                   "remove <email>",
	Short:                 "remove an administrator from the account",
	Long:                  "*Requires admin permissions.*\nUpdates the application Cloudformation stack to remove an administrator.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		email := args[0]
		startSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		cfnSvc := cloudformation.New(sess)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: aws.String(accountStackName),
		})
		checkErr(err)
		stack := stackOutput.Stacks[0]
		usersCSV, err := parameterValue(stack, "Administrators")
		checkErr(err)
		userList := splitAndTrimCSV(usersCSV)
		idx := indexOf(userList, email)
		if idx < 0 {
			checkErr(fmt.Errorf("%s is not an administrator", email))
		}
		newUsersCSV := strings.Join(append(userList[:idx], userList[idx+1:]...), ",")
		err = replaceParameter(stack, "Administrators", &newUsersCSV)
		checkErr(err)
		stack, err = updateStackAndWait(sess, &cloudformation.UpdateStackInput{
			StackName:           stack.StackName,
			Parameters:          stack.Parameters,
			UsePreviousTemplate: aws.Bool(true),
			Capabilities:        []*string{aws.String("CAPABILITY_IAM")},
		})
		checkErr(err)
		if *stack.StackStatus != cloudformation.StackStatusUpdateComplete {
			checkErr(fmt.Errorf("stack did not update successfully -- %s", *stack.StackStatus))
		}
		Spinner.Stop()
		printSuccess(fmt.Sprintf("%s removed as an administrator", email))
	},
}

func init() {
	rootCmd.AddCommand(adminsCmd)

	adminsCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", "AWS account ID or alias (not needed if you are only the administrator of one account)")
	adminsCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")

	adminsCmd.AddCommand(adminsAddCmd)
	adminsCmd.AddCommand(adminsRemoveCmd)
}
