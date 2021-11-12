/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

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

	"github.com/apppackio/apppack/app"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

// openCmd represents the open command
var openCmd = &cobra.Command{
	Use:   "open",
	Short: "open the app in a browser",
	Run: func(cmd *cobra.Command, args []string) {
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		u, err := a.URL(nil)
		checkErr(err)
		fmt.Printf("opening %s\n", aurora.Bold(*u))
		browser.OpenURL(*u)
	},
}

func init() {
	rootCmd.AddCommand(openCmd)
	openCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	openCmd.MarkPersistentFlagRequired("app-name")
	openCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
}
