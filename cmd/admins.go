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

	"github.com/apppackio/apppack/stacks"
	"github.com/apppackio/apppack/stringslice"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/spf13/cobra"
)

func accountStack(sess *session.Session) (*stacks.AccountStack, error) {
	stack := &stacks.AccountStack{Parameters: &stacks.AccountStackParameters{}}
	err := stacks.LoadStackFromCloudformation(sess, stack, new(string))
	if err != nil {
		return nil, err
	}
	return stack, nil
}

func updateAdministrators(sess *session.Session, stack *stacks.AccountStack, name *string) error {
	sort.Strings(stack.Parameters.Administrators)
	ui.StartSpinner()
	if err := stacks.ModifyStack(sess, stack, name); err != nil {
		ui.Spinner.Stop()
		return err
	}
	ui.Spinner.Stop()
	printSuccess("Account administrators updated")
	for _, u := range stack.Parameters.Administrators {
		fmt.Printf("  • %s\n", u)
	}
	return nil
}

// accessCmd represents the access command
var adminsCmd = &cobra.Command{
	Use:                   "admins",
	Short:                 "list the administrators for an account",
	Long:                  "*Requires admin permissions.*",
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := accountStack(sess)
		checkErr(err)
		ui.Spinner.Stop()
		for _, u := range stack.Parameters.Administrators {
			fmt.Println(u)
		}
	},
}

// adminsAddCmd represents the admins add command
var adminsAddCmd = &cobra.Command{
	Use:                   "add <email>...",
	Short:                 "add administrators to the account",
	Long:                  "*Requires admin permissions.*\nUpdates the account Cloudformation stack to add administrators.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MinimumNArgs(1),
	Example:               "apppack admins add user1@example.com user2@example.com",
	Run: func(cmd *cobra.Command, args []string) {
		for _, email := range args {
			if !validateEmail(email) {
				checkErr(fmt.Errorf("%s does not appear to be a valid email address", email))
			}
		}
		ui.StartSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := accountStack(sess)
		checkErr(err)
		stack.Parameters.Administrators = append(stack.Parameters.Administrators, args...)
		var dupes []string
		stack.Parameters.Administrators, dupes = stringslice.Deduplicate(stack.Parameters.Administrators)
		ui.Spinner.Stop()
		for _, d := range dupes {
			printWarning(fmt.Sprintf("%s is already an administrator", d))
		}
		checkErr(updateAdministrators(sess, stack, &AppName))
	},
}

// adminsRemoveCmd represents the admins remove command
var adminsRemoveCmd = &cobra.Command{
	Use:   "remove <email>",
	Short: "remove an administrator from the account",
	Long: `*Requires admin permissions.*
Updates the application Cloudformation stack to remove an administrators.`,
	DisableFlagsInUseLine: true,
	Args:                  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, email := range args {
			if !validateEmail(email) {
				checkErr(fmt.Errorf("%s does not appear to be a valid email address", email))
			}
		}
		ui.StartSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stack, err := accountStack(sess)
		checkErr(err)
		var notFound []string
		stack.Parameters.Administrators, notFound = removeFromSlice(stack.Parameters.Administrators, args)
		ui.Spinner.Stop()
		for _, n := range notFound {
			printWarning(fmt.Sprintf("%s is not an administrator", n))
		}
		checkErr(updateAdministrators(sess, stack, &AppName))
	},
}

func init() {
	rootCmd.AddCommand(adminsCmd)

	adminsCmd.PersistentFlags().StringVarP(
		&AccountIDorAlias,
		"account",
		"c",
		"",
		"AWS account ID or alias (not needed if you are only the administrator of one account)",
	)
	adminsCmd.PersistentFlags().BoolVar(
		&UseAWSCredentials,
		"aws-credentials",
		false,
		"use AWS credentials instead of AppPack.io federation",
	)

	adminsCmd.AddCommand(adminsAddCmd)
	adminsCmd.AddCommand(adminsRemoveCmd)
}
