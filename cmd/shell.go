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

	"github.com/lincolnloop/apppack/app"

	"github.com/spf13/cobra"
)

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open an interactive shell in the remote environment",
	Long:  `Open an interactive shell in the remote environment`,
	Run: func(cmd *cobra.Command, args []string) {
		Spinner.Start()
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
		Spinner.Suffix = fmt.Sprintf(" starting task %s", *shellTask.TaskArn)
		err = a.WaitForTaskRunning(shellTask)
		checkErr(err)
		Spinner.Stop()
		err = a.ConnectToTask(shellTask, &a.Settings.Shell.Command)
		checkErr(err)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "App name (required)")
	shellCmd.MarkPersistentFlagRequired("app-name")
}
