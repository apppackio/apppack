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

	"github.com/apppackio/apppack/auth"
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
		pauseUntilEnter("Finish verification in your web browser then press ENTER to continue.")
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
		startSpinner()
		apps, err := auth.AppList()
		checkErr(err)
		appGroups := make(map[string][]*auth.AppRole)
		pipelineGroups := make(map[string][]*auth.AppRole)
		for _, app := range apps {
			key := fmt.Sprintf("%s%s", app.AccountID, app.Region)
			if app.Pipeline {
				val, ok := pipelineGroups[key]
				if ok {
					pipelineGroups[key] = append(val, app)
				} else {
					pipelineGroups[key] = []*auth.AppRole{app}
				}
			} else {
				val, ok := appGroups[key]
				if ok {
					appGroups[key] = append(val, app)
				} else {
					appGroups[key] = []*auth.AppRole{app}
				}
			}
		}
		Spinner.Stop()
		if len(appGroups) > 0 {
			fmt.Printf("%s %s\n", aurora.Faint("==="), aurora.White("Apps"))
			for _, group := range appGroups {
				fmt.Println(aurora.Faint(fmt.Sprintf("Account: %s (%s)", group[0].AccountID, group[0].Region)))
				for _, app := range group {
					fmt.Printf("  %s\n", aurora.Green(app.AppName))
				}
			}
		}
		if len(appGroups) > 0 && len(pipelineGroups) > 0 {
			fmt.Print("\n")
		}
		if len(pipelineGroups) > 0 {
			fmt.Printf("%s %s\n", aurora.Faint("==="), aurora.White("Pipelines"))
			for _, group := range pipelineGroups {
				fmt.Println(aurora.Faint(fmt.Sprintf("Account: %s (%s)", group[0].AccountID, group[0].Region)))
				for _, app := range group {
					fmt.Printf("  %s\n", aurora.Green(app.AppName))
				}
			}
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
