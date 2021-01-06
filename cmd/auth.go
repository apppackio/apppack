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
	"bufio"
	"fmt"
	"os"

	"github.com/lincolnloop/apppack/auth"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

// authCmd represents the auth command
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "authenticate with AppPack.io",
	Long: `This will open a web browser and/or provide a URL to visit to verify this device.
	
Your credentials are cached locally for your user, so these commands should not be used on a shared device account.`,
	DisableFlagsInUseLine: true,
}

// loginCmd represents the login command
var loginCmd = &cobra.Command{
	Use:                   "login",
	Short:                 "login to AppPack.io on this device",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		dataPtr, err := auth.LoginInit()
		checkErr(err)
		data := *dataPtr
		fmt.Println("Your verification code is", data.UserCode)
		browser.OpenURL(data.VerificationURIComplete)
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Finish verification in your web browser then press ENTER to continue.")
		_, _ = reader.ReadString('\n')
		userInfo, err := auth.LoginComplete(data.DeviceCode)
		checkErr(err)
		printSuccess(fmt.Sprintf("Logged in as %s", aurora.Bold(userInfo.Email)))
	},
}

// logoutCmd represents the logout command
var logoutCmd = &cobra.Command{
	Use:                   "logout",
	Short:                 "logout of AppPack.io on this device",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		err := auth.Logout()
		checkErr(err)
		printSuccess("Logged out.")
	},
}

// whoAmICmd represents the whoami command
var whoAmICmd = &cobra.Command{
	Use:                   "whoami",
	Short:                 "show login information for the current user",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		email, err := auth.WhoAmI()
		checkErr(err)
		printSuccess(fmt.Sprintf("You are currently logged in as %s", aurora.Bold(*email)))
	},
}

// appsCmd represents the apps command
var appsCmd = &cobra.Command{
	Use:                   "apps",
	Short:                 "list the apps you have access to",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		apps, err := auth.AppList()
		checkErr(err)
		for _, app := range apps {
			fmt.Printf("%s\t%s\n", app.AppName, aurora.Faint(fmt.Sprintf("%s account:%s", app.Region, app.AccountID)))
		}
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(loginCmd)
	authCmd.AddCommand(logoutCmd)
	authCmd.AddCommand(whoAmICmd)
	authCmd.AddCommand(appsCmd)
}
