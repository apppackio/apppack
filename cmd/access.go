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
	"regexp"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/spf13/cobra"
)

func parameterValue(stack *cloudformation.Stack, key string) (*string, error) {
	for _, p := range stack.Parameters {
		if *p.ParameterKey == key {
			return p.ParameterValue, nil
		}
	}
	return nil, fmt.Errorf("Cloudformation parameter %s not found", key)
}

func replaceParameter(stack *cloudformation.Stack, key string, value *string) error {
	for _, p := range stack.Parameters {
		if *p.ParameterKey == key {
			p.ParameterValue = value
			return nil
		}
	}
	return fmt.Errorf("Cloudformation parameter %s not found", key)
}

func validateEmail(email string) bool {
	pattern := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
	return pattern.MatchString(email)
}

func splitAndTrimCSV(csv *string) []string {
	var items []string
	for _, i := range strings.Split(*csv, ",") {
		items = append(items, strings.Trim(i, " "))
	}
	return items
}

func indexOf(arr []string, item string) int {
	for k, v := range arr {
		if item == v {
			return k
		}
	}
	return -1
}

// accessCmd represents the access command
var accessCmd = &cobra.Command{
	Use:   "access",
	Short: "list of users with access to the app",
	Long:  `A llist of uses with access to the app.`,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		sess := session.Must(session.NewSession())
		cfnSvc := cloudformation.New(sess)
		stackName := fmt.Sprintf(appStackName, AppName)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		checkErr(err)
		usersCSV, err := parameterValue(stackOutput.Stacks[0], "AllowedUsers")
		checkErr(err)
		users := splitAndTrimCSV(usersCSV)
		sort.Strings(users)
		Spinner.Stop()
		for _, u := range users {
			fmt.Println(u)
		}
	},
}

// accessAddCmd represents the access command
var accessAddCmd = &cobra.Command{
	Use:   "add [EMAIL]",
	Short: "add access for a user to the app",
	Long:  `Add access for a user to the app.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		email := args[0]
		if !validateEmail(email) {
			checkErr(fmt.Errorf("%s does not appear to be a valid email address", email))
		}
		startSpinner()
		sess := session.Must(session.NewSession())
		cfnSvc := cloudformation.New(sess)
		stackName := fmt.Sprintf(appStackName, AppName)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		checkErr(err)
		stack := stackOutput.Stacks[0]
		usersCSV, err := parameterValue(stack, "AllowedUsers")
		checkErr(err)
		usersCSV = aws.String(strings.Join([]string{*usersCSV, email}, ","))
		err = replaceParameter(stack, "AllowedUsers", usersCSV)
		checkErr(err)
		_, err = updateStackAndWait(sess, &cloudformation.UpdateStackInput{
			StackName:           &stackName,
			Parameters:          stack.Parameters,
			UsePreviousTemplate: aws.Bool(true),
			Capabilities:        []*string{aws.String("CAPABILITY_IAM")},
		})
		checkErr(err)
		Spinner.Stop()
		printSuccess(fmt.Sprintf("access added for %s on %s", email, AppName))
	},
}

// accessRemoveCmd represents the access command
var accessRemoveCmd = &cobra.Command{
	Use:   "remove [EMAIL]",
	Short: "remove access for a user to the app",
	Long:  `Remove access for a user to the app.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		email := args[0]
		startSpinner()
		sess := session.Must(session.NewSession())
		cfnSvc := cloudformation.New(sess)
		stackName := fmt.Sprintf(appStackName, AppName)
		stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		checkErr(err)
		stack := stackOutput.Stacks[0]
		usersCSV, err := parameterValue(stack, "AllowedUsers")
		userList := splitAndTrimCSV(usersCSV)
		idx := indexOf(userList, email)
		if idx < 0 {
			checkErr(fmt.Errorf("%s does not have access to %s", email, AppName))
		}
		newUsersCSV := strings.Join(append(userList[:idx], userList[idx+1:]...), ",")
		err = replaceParameter(stack, "AllowedUsers", &newUsersCSV)
		checkErr(err)
		_, err = updateStackAndWait(sess, &cloudformation.UpdateStackInput{
			StackName:           &stackName,
			Parameters:          stack.Parameters,
			UsePreviousTemplate: aws.Bool(true),
			Capabilities:        []*string{aws.String("CAPABILITY_IAM")},
		})
		checkErr(err)
		Spinner.Stop()
		printSuccess(fmt.Sprintf("access removed for %s on %s", email, AppName))
	},
}

func init() {
	rootCmd.AddCommand(accessCmd)

	accessCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "App name (required)")
	accessCmd.MarkPersistentFlagRequired("app-name")

	accessCmd.AddCommand(accessAddCmd)
	accessCmd.AddCommand(accessRemoveCmd)
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// accessCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// accessCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
