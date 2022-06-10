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
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/apppackio/apppack/version"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/cli/safeexec"
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
			printError("Failed to get latest release")
			return
		}
		if version.IsUpToDate(&latest) {
			var displayVersion string
			if version.Environment != "production" {
				displayVersion = version.Environment
			} else {
				displayVersion = version.Version
			}
			printWarning(fmt.Sprintf("Already up to date. Version %s", displayVersion))
			return
		}
		printWarning(fmt.Sprintf("Upgrading %s -> %s", version.Version, latest.Name))

		cmdFactory := factory.New(latest.Name)

		// We could run an automatic upgrade here if we wanted
		isHomebrew := isUnderHomebrew(cmdFactory.Executable())
		if isHomebrew {
			printSuccess(fmt.Sprintf("To upgrade, run: %s\n", "brew update && brew upgrade apppack"))
		} else {
			printSuccess(fmt.Sprintf("To upgrade, follow the instructions at: %s\n", "https://docs.apppack.io/how-to/install/"))
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.AddCommand(upgradeCliCmd)
}

// Check whether the gh binary was found under the Homebrew prefix
func isUnderHomebrew(ghBinary string) bool {
	brewExe, err := safeexec.LookPath("brew")
	if err != nil {
		return false
	}

	brewPrefixBytes, err := exec.Command(brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)
	return strings.HasPrefix(ghBinary, brewBinPrefix)
}
