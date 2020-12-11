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
	. "github.com/logrusorgru/aurora"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

// authCmd represents the auth command
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with AppPack.io",
	Long: `This will open a web browser and/or provide a URL to visit to verify this device.
	
Your credentials are cached locally for your user, so these commands should not be used on a shared device account.`,
}

// loginCmd represents the login command
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to AppPack.io on this device",
	Long:  `Login to AppPack.io on this device`,
	Run: func(cmd *cobra.Command, args []string) {
		dataPtr, err := auth.LoginInit()
		checkErr(err)
		data := *dataPtr
		fmt.Println("Your verification code is", data["user_code"])
		browser.OpenURL(data["verification_uri_complete"])
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Finish verification in your web browser then press ENTER to continue.")
		_, _ = reader.ReadString('\n')
		userInfo, err := auth.LoginComplete(data["device_code"])
		checkErr(err)
		fmt.Println(Green(fmt.Sprintf("Logged in as %s", (*userInfo).Email)))
	},
}

// logoutCmd represents the logout command
var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		auth.Logout()
		fmt.Println(Green(fmt.Sprintf("Logged out.")))
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(loginCmd)
	authCmd.AddCommand(logoutCmd)
}
