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
	"github.com/apppackio/apppack/version"
	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:                   "version",
	Short:                 "show the version of the apppack command",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		if version.Environment != "production" {
			fmt.Println(version.Environment)
		} else {
			fmt.Println(version.Version)
		}
	},
}

// loginCmd represents the login command
var upgradeCliCmd = &cobra.Command{
	Use:                   "upgrade",
	Short:                 "upgrade to apppack to the latest version",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		latest, err := version.GetLatestRelease()
		if err != nil {
			return
		}
		if version.IsUpToDate(&latest) {
			printWarning(fmt.Sprintf("Already up to date. Version %s", latest.Name))
			return
		}
		printSuccess(fmt.Sprintf("upgrading to %s", latest))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.AddCommand(upgradeCliCmd)
}
