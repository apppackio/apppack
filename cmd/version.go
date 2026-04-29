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
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/apppackio/apppack/selfupdate"
	"github.com/apppackio/apppack/ui"
	"github.com/apppackio/apppack/version"
	"github.com/cli/safeexec"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

var forceUpdate bool

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:                   "version",
	Short:                 "show the version of the apppack command",
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		if version.Environment != "production" {
			fmt.Println(version.Environment)
		} else {
			fmt.Println(version.Version)
		}
	},
}

// versionCheckCmd checks if a newer version is available
var versionCheckCmd = &cobra.Command{
	Use:                   "check",
	Short:                 "check if a newer version is available",
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		ctx := context.Background()
		ui.StartSpinner()
		ui.Spinner.Suffix = " checking for updates..."

		release, err := version.GetLatestReleaseInfo(ctx, http.DefaultClient, repo)
		checkErr(err)

		ui.Spinner.Stop()

		if version.VersionGreaterThan(release.Version, version.Version) {
			fmt.Printf("%s %s → %s\n",
				aurora.Yellow("Update available:"),
				aurora.Cyan(strings.TrimPrefix(version.Version, "v")),
				aurora.Cyan(strings.TrimPrefix(release.Version, "v")),
			)
			fmt.Printf("Run %s to update\n", aurora.White("apppack version update"))
		} else {
			printSuccess(fmt.Sprintf("Already up to date (version %s)", strings.TrimPrefix(version.Version, "v")))
		}
	},
}

// versionUpdateCmd updates apppack to the latest version
var versionUpdateCmd = &cobra.Command{
	Use:                   "update",
	Short:                 "update apppack to the latest version",
	Long:                  "Downloads and installs the latest version of apppack from GitHub releases.",
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		ctx := context.Background()

		// Get current binary path
		appPath, err := safeexec.LookPath(os.Args[0])
		checkErr(err)

		// Block Homebrew installs
		if IsUnderHomebrew(appPath) {
			printWarning("AppPack was installed via Homebrew")
			fmt.Printf("To update, run: %s\n", aurora.White("brew upgrade apppack"))

			return
		}

		ui.StartSpinner()
		ui.Spinner.Suffix = " checking for updates..."

		release, err := version.GetLatestReleaseInfo(ctx, http.DefaultClient, repo)
		checkErr(err)

		// Check if update is needed
		if !forceUpdate && !version.VersionGreaterThan(release.Version, version.Version) {
			ui.Spinner.Stop()
			printSuccess(fmt.Sprintf("Already up to date (version %s)", strings.TrimPrefix(version.Version, "v")))

			return
		}

		ui.Spinner.Suffix = fmt.Sprintf(" downloading %s...", release.Version)

		err = selfupdate.Update(ctx, http.DefaultClient, release, appPath)
		checkErr(err)

		ui.Spinner.Stop()
		printSuccess(fmt.Sprintf("Updated to version %s", strings.TrimPrefix(release.Version, "v")))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.AddCommand(versionCheckCmd)
	versionCmd.AddCommand(versionUpdateCmd)
	versionUpdateCmd.Flags().BoolVarP(&forceUpdate, "force", "f", false, "force update even if already on latest version")
}
