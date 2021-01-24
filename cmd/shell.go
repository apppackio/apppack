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

	"github.com/apppackio/apppack/app"

	"github.com/spf13/cobra"
)

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "open an interactive shell in the remote environment",
	Long: `Open an interactive shell in the remote environment

Requires installation of Amazon's SSM Session Manager. https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html`,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		err = a.LoadSettings()
		checkErr(err)
		taskOutput, err := a.StartTask(
			&a.Settings.Shell.TaskFamily,
			app.ShellBackgroundCommand,
			false,
		)
		checkErr(err)
		shellTask := taskOutput.Tasks[0]
		checkErr(err)
		Spinner.Stop()
		fmt.Printf("starting %s\n", *shellTask.TaskArn)
		startSpinner()
		err = a.WaitForTaskRunning(shellTask)
		checkErr(err)
		Spinner.Stop()
		err = a.ConnectToTask(shellTask, &a.Settings.Shell.Command)
		checkErr(err)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	shellCmd.MarkPersistentFlagRequired("app-name")
}
