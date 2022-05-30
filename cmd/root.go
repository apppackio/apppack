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
	"github.com/apppackio/apppack/version"
	"os"
	"strings"

	"github.com/apppackio/apppack/auth"
	"github.com/apppackio/apppack/ui"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var debug bool

const (
	timeFmt = "Jan 02, 2006 15:04:05 -0700"
)

// AppName is the `--app-name` flag
var AppName string

// AccountIDorAlias is the `--account` flag
var AccountIDorAlias string
var UseAWSCredentials = false
var SessionDurationSeconds = 900
var MaxSessionDurationSeconds = 3600

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:                   "apppack",
	Short:                 "the CLI interface to AppPack.io",
	Long:                  `AppPack is a tool to manage applications deployed on AWS via AppPack.io`,
	DisableAutoGenTag:     true,
	DisableFlagsInUseLine: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if debug {
			logrus.SetOutput(os.Stdout)
			logrus.SetLevel(logrus.DebugLevel)
		} else {
			logrus.SetLevel(logrus.ErrorLevel)
		}
		if !version.IsUpToDate(nil) { // ToDo: Only run after X minutes?
			printWarning(fmt.Sprintf("Time to upgrade!\nRun \"apppack version upgrade\" to upgrade apppack to the latest version"))
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
}

func checkErr(err error) {
	if err == nil {
		return
	}
	ui.Spinner.Stop()
	if strings.HasPrefix(err.Error(), auth.TokenRefreshErr) {
		fmt.Println(
			aurora.Yellow(fmt.Sprintf("⚠  %s", auth.TokenRefreshErr)),
			aurora.Faint(strings.TrimPrefix(err.Error(), fmt.Sprintf("%s: ", auth.TokenRefreshErr))),
		)
		fmt.Printf("%s Reauthenticate this device by running: %s\n", aurora.Blue("ℹ"), aurora.White("apppack auth login"))
	} else {
		printError(err.Error())
	}
	os.Exit(1)
}

func printError(text string) {
	fmt.Println(aurora.Red(fmt.Sprintf("✖ %s", text)))
}

func printSuccess(text string) {
	fmt.Println(aurora.Green(fmt.Sprintf("✔ %s", text)))
}

func printWarning(text string) {
	fmt.Println(aurora.Yellow(fmt.Sprintf("⚠  %s", text)))
}

func confirmAction(message, text string) {
	printWarning(fmt.Sprintf("%s\n   Are you sure you want to continue?", message))
	fmt.Printf("\nType %s to confirm.\n%s ", aurora.White(text), aurora.White(">"))
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != text {
		checkErr(fmt.Errorf("aborting due to user input"))
	}
}
