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

	"github.com/apppackio/apppack/auth"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
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
	return nil, fmt.Errorf("cloudformation parameter %s not found", key)
}

func replaceParameter(stack *cloudformation.Stack, key string, value *string) error {
	for _, p := range stack.Parameters {
		if *p.ParameterKey == key {
			p.ParameterValue = value
			return nil
		}
	}
	return fmt.Errorf("cloudformation parameter %s not found", key)
}

func validateEmail(email string) bool {
	pattern := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
	return pattern.MatchString(email)
}

func splitAndTrimCSV(csv *string) []string {
	var items []string
	for _, i := range strings.Split(*csv, ",") {
		i = strings.TrimSpace(i)
		if i != "" {
			items = append(items, i)
		}
	}
	return items
}

// deduplicate removes duplicates from a slice of strings
func deduplicate(slice []string) ([]string, []string) {
	seen := make(map[string]bool)
	var result []string
	var dupes []string
	for _, s := range slice {
		if seen[s] {
			dupes = append(dupes, s)
			continue
		}
		seen[s] = true
		result = append(result, s)
	}
	return result, dupes
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// removeFromList removes items in a slice from a slice of strings
// it returns both the new slice and a slice of items not found
func removeFromSlice(slice, toRemove []string) ([]string, []string) {
	var result []string
	var notFound []string
	for _, r := range toRemove {
		if !stringInSlice(r, slice) {
			notFound = append(notFound, r)
		}
	}
	for _, s := range slice {
		if !stringInSlice(s, toRemove) {
			result = append(result, s)
		}
	}
	return result, notFound
}

func indexOf(arr []string, item string) int {
	for k, v := range arr {
		if item == v {
			return k
		}
	}
	return -1
}

func appOrPipelineStack(sess *session.Session, name string) (*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(sess)
	stackName := appStackName(name)
	stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err == nil {
		return stackOutput.Stacks[0], nil
	}
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "ValidationError" {
			stackName = pipelineStackName(name)
			stackOutput, err = cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
				StackName: &stackName,
			})
			if err == nil {
				return stackOutput.Stacks[0], nil
			}
		}
	}
	return nil, err
}

func adminSession(sessionDuration int) (*session.Session, error) {
	if UseAWSCredentials {
		if region != "" {
			return session.NewSession(&aws.Config{Region: &region})
		}
		sess, err := session.NewSession()
		if err != nil {
			return nil, err
		}
		if *sess.Config.Region == "" {
			return nil, fmt.Errorf("no region provided. Use the `--region` flag or set the AWS_REGION environment")
		}
		return sess, nil
	}
	sess, _, err := auth.AdminAWSSession(AccountIDorAlias, sessionDuration)
	return sess, err
}

func updateAllowedUsers(sess *session.Session, stack *cloudformation.Stack, users []string) error {
	startSpinner()
	sort.Strings(users)
	usersCSV := aws.String(strings.Join(users, ","))
	if err := replaceParameter(stack, "AllowedUsers", usersCSV); err != nil {
		return err
	}
	_, err := updateStackAndWait(sess, &cloudformation.UpdateStackInput{
		StackName:           stack.StackName,
		Parameters:          stack.Parameters,
		UsePreviousTemplate: aws.Bool(true),
		Capabilities:        []*string{aws.String("CAPABILITY_IAM")},
	})
	if err != nil {
		return err
	}
	Spinner.Stop()
	printSuccess(fmt.Sprintf("allowed users updated for %s", AppName))
	for _, u := range users {
		fmt.Printf("  • %s\n", u)
	}
	return nil
}

// accessCmd represents the access command
var accessCmd = &cobra.Command{
	Use:                   "access",
	Short:                 "list users with access to the app",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		var err error
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := appOrPipelineStack(sess, AppName)
		checkErr(err)
		usersCSV, err := parameterValue(stack, "AllowedUsers")
		checkErr(err)
		users := splitAndTrimCSV(usersCSV)
		sort.Strings(users)
		Spinner.Stop()
		for _, u := range users {
			fmt.Printf("  • %s\n", u)
		}
	},
}

// accessAddCmd represents the access command
var accessAddCmd = &cobra.Command{
	Use:                   "add <email>...",
	Short:                 "add access for users to the app",
	Long:                  "*Requires admin permissions.*\nUpdates the application Cloudformation stack to add access for the user.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MinimumNArgs(1),
	Example:               "apppack -a my-app access add user1@example.com user2@example.com",
	Run: func(cmd *cobra.Command, args []string) {
		for _, email := range args {

			if !validateEmail(email) {
				checkErr(fmt.Errorf("%s does not appear to be a valid email address", email))
			}
		}
		startSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := appOrPipelineStack(sess, AppName)
		checkErr(err)
		usersCSV, err := parameterValue(stack, "AllowedUsers")
		checkErr(err)
		users := splitAndTrimCSV(usersCSV)
		users = append(users, args...)
		users, dupes := deduplicate(users)
		Spinner.Stop()
		for _, d := range dupes {
			printWarning(fmt.Sprintf("%s already has access to %s", d, AppName))
		}
		checkErr(updateAllowedUsers(sess, stack, users))
	},
}

// accessRemoveCmd represents the access command
var accessRemoveCmd = &cobra.Command{
	Use:                   "remove <email>...",
	Short:                 "remove access for users to the app",
	Long:                  "*Requires admin permissions.*\nUpdates the application Cloudformation stack to remove access for the user.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MinimumNArgs(1),
	Example:               "apppack -a my-app access remove user1@example.com user2@example.com",
	Run: func(cmd *cobra.Command, args []string) {
		for _, email := range args {
			if !validateEmail(email) {
				checkErr(fmt.Errorf("%s does not appear to be a valid email address", email))
			}
		}
		startSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := appOrPipelineStack(sess, AppName)
		checkErr(err)
		usersCSV, err := parameterValue(stack, "AllowedUsers")
		checkErr(err)
		userList := splitAndTrimCSV(usersCSV)
		users, notFound := removeFromSlice(userList, args)
		Spinner.Stop()
		for _, n := range notFound {
			printWarning(fmt.Sprintf("%s does not have access to %s", n, AppName))
		}
		checkErr(updateAllowedUsers(sess, stack, users))
	},
}

func init() {
	rootCmd.AddCommand(accessCmd)

	accessCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	accessCmd.MarkPersistentFlagRequired("app-name")
	accessCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", "AWS account ID or alias (not needed if you are only the administrator of one account)")
	accessCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")

	accessCmd.AddCommand(accessAddCmd)
	accessAddCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region of app")
	accessCmd.AddCommand(accessRemoveCmd)
	accessRemoveCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region of app")
}
