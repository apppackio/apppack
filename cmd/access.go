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

	"github.com/apppackio/apppack/auth"
	"github.com/apppackio/apppack/stacks"
	"github.com/apppackio/apppack/stringslice"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/spf13/cobra"
)

var CurrentAccountRole *auth.AdminRole

func validateEmail(email string) bool {
	pattern := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
	return pattern.MatchString(email)
}

// removeFromList removes items in a slice from a slice of strings
// it returns both the new slice and a slice of items not found
func removeFromSlice(slice, toRemove []string) ([]string, []string) {
	var result []string
	var notFound []string
	for _, r := range toRemove {
		if !stringslice.Contains(r, slice) {
			notFound = append(notFound, r)
		}
	}
	for _, s := range slice {
		if !stringslice.Contains(s, toRemove) {
			result = append(result, s)
		}
	}
	return result, notFound
}

func appOrPipelineStack(sess *session.Session, name string) (*stacks.AppStack, error) {
	stack := stacks.AppStack{Pipeline: false, Parameters: &stacks.AppStackParameters{}}
	err := stacks.LoadStackFromCloudformation(sess, &stack, &name)
	if err != nil {
		stack.Pipeline = true
		err = stacks.LoadStackFromCloudformation(sess, &stack, &name)
		if err != nil {
			return nil, err
		}
	}
	return &stack, nil
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
	var sess *session.Session
	var err error
	sess, CurrentAccountRole, err = auth.AdminAWSSession(AccountIDorAlias, sessionDuration, region)
	return sess, err
}

func updateAllowedUsers(sess *session.Session, stack *stacks.AppStack, name *string) error {
	ui.StartSpinner()
	if err := stacks.ModifyStack(sess, stack, name); err != nil {
		ui.Spinner.Stop()
		return err
	}
	ui.Spinner.Stop()
	printSuccess(fmt.Sprintf("allowed users updated for %s", AppName))
	for _, u := range stack.Parameters.AllowedUsers {
		fmt.Printf("  • %s\n", u)
	}
	return nil
}

// accessCmd represents the access command
var accessCmd = &cobra.Command{
	Use:                   "access",
	Short:                 "list users with access to the app",
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		var err error
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := appOrPipelineStack(sess, AppName)
		checkErr(err)
		sort.Strings(stack.Parameters.AllowedUsers)
		ui.Spinner.Stop()
		for _, u := range stack.Parameters.AllowedUsers {
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
		ui.StartSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := appOrPipelineStack(sess, AppName)
		checkErr(err)
		stack.Parameters.AllowedUsers = append(stack.Parameters.AllowedUsers, args...)
		var dupes []string
		stack.Parameters.AllowedUsers, dupes = stringslice.Deduplicate(stack.Parameters.AllowedUsers)
		sort.Strings(stack.Parameters.AllowedUsers)
		ui.Spinner.Stop()
		for _, d := range dupes {
			printWarning(fmt.Sprintf("%s already has access to %s", d, AppName))
		}
		checkErr(updateAllowedUsers(sess, stack, &AppName))
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
		ui.StartSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := appOrPipelineStack(sess, AppName)
		checkErr(err)
		var notFound []string
		stack.Parameters.AllowedUsers, notFound = removeFromSlice(stack.Parameters.AllowedUsers, args)
		sort.Strings(stack.Parameters.AllowedUsers)
		ui.Spinner.Stop()
		for _, n := range notFound {
			printWarning(fmt.Sprintf("%s does not have access to %s", n, AppName))
		}
		checkErr(updateAllowedUsers(sess, stack, &AppName))
	},
}

func init() {
	rootCmd.AddCommand(accessCmd)

	accessCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	accessCmd.MarkPersistentFlagRequired("app-name")
	accessCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", "AWS account ID or alias (not needed if you are only the administrator of one account)")
	accessCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	accessCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region to use (defaults to the region of the account)")

	accessCmd.AddCommand(accessAddCmd)
	accessAddCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region of app")
	accessCmd.AddCommand(accessRemoveCmd)
	accessRemoveCmd.PersistentFlags().StringVar(&region, "region", "", "AWS region of app")
}
