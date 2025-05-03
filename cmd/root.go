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
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/apppackio/apppack/auth"
	"github.com/apppackio/apppack/state"
	"github.com/apppackio/apppack/ui"
	"github.com/apppackio/apppack/version"
	"github.com/cli/safeexec"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const timeFmt = "Jan 02, 2006 15:04:05 -0700"

var (
	// debug is the `--debug` flag
	debug bool
	// AppName is the `--app-name` flag
	AppName string
	// AsJSON is the `--json` flag
	AsJSON bool
	// AccountIDorAlias is the `--account` flag
	AccountIDorAlias          string
	UseAWSCredentials         = false
	SessionDurationSeconds    = 900
	MaxSessionDurationSeconds = 3600
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:                   "apppack",
	Short:                 "the CLI interface to AppPack.io",
	Long:                  `AppPack is a tool to manage applications deployed on AWS via AppPack.io`,
	DisableAutoGenTag:     true,
	DisableFlagsInUseLine: true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		if debug {
			logrus.SetOutput(os.Stdout)
			logrus.SetLevel(logrus.DebugLevel)
		} else {
			logrus.SetLevel(logrus.ErrorLevel)
		}
	},
}

// This code is partly cherry-picked from https://github.com/cli/cli/blob/82927b0cc2a831adda22b0a7bf43938bd15e1126/main.go
// It is licensed under the MIT license https://github.com/cli/cli/blob/82927b0cc2a831adda22b0a7bf43938bd15e1126/LICENSE
var repo = "apppackio/apppack"

func checkForUpdate(ctx context.Context, currentVersion string) (*version.ReleaseInfo, error) {
	userCacheDir, err := state.CacheDir()
	if err != nil {
		return nil, err
	}
	stateFilePath := filepath.Join(userCacheDir, "version.json")
	return version.CheckForUpdate(ctx, http.DefaultClient, stateFilePath, repo, currentVersion)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	retcode := 0
	defer func() { os.Exit(retcode) }()

	// start update check process in the background
	ctx := context.Background()
	updateCtx, updateCancel := context.WithCancel(ctx)

	defer updateCancel()
	updateMessageChan := make(chan *version.ReleaseInfo)

	go func() {
		rel, err := checkForUpdate(updateCtx, version.Version)
		if err != nil {
			logrus.WithFields(logrus.Fields{"error": err}).Debug("checking for update failed")
		}
		updateMessageChan <- rel
	}()

	// run command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		retcode = 1
		return
	}

	updateCancel() // if the update checker hasn't completed by now, abort it
	newRelease := <-updateMessageChan
	if newRelease != nil {
		printUpdateMessage(newRelease)
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

func printUpdateMessage(newRelease *version.ReleaseInfo) {
	appPath, err := exec.LookPath(os.Args[0])
	checkErr(err)
	isHomebrew := isUnderHomebrew(appPath)
	fmt.Fprintf(os.Stderr, "\n\n%s %s → %s\n",
		aurora.Yellow("A new release of apppack is available:"),
		aurora.Cyan(strings.TrimPrefix(version.Version, "v")),
		aurora.Cyan(strings.TrimPrefix(newRelease.Version, "v")),
	)
	if isHomebrew {
		fmt.Fprintf(os.Stderr, "To upgrade, run: %s\n", "brew upgrade apppack")
	}
	fmt.Fprintf(os.Stderr, "%s\n\n", aurora.Yellow(newRelease.URL))
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

// Check whether the apppack binary was found under the Homebrew prefix
func isUnderHomebrew(apppackBinary string) bool {
	brewExe, err := safeexec.LookPath("brew")
	if err != nil {
		return false
	}
	brewPrefixBytes, err := exec.Command(brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)
	return strings.HasPrefix(apppackBinary, brewBinPrefix)
}
