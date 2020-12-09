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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/lincolnloop/apppack/app"
	"github.com/spf13/cobra"
)

// dbCmd represents the db command
var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("db called")
	},
}

// dbShellCmd represents the db shell command
var dbShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		Spinner.Start()
		app, err := app.Init(AppName)
		checkErr(err)
		err = app.LoadSettings()
		checkErr(err)
		taskOutput, err := app.StartShellTask(&app.Settings.DBUtils.ShellTaskFamily)
		checkErr(err)
		shellTask := taskOutput.Tasks[0]
		checkErr(err)
		Spinner.Suffix = fmt.Sprintf(" starting task %s", *shellTask.TaskArn)
		ecsSvc := ecs.New(app.Session)
		err = ecsSvc.WaitUntilTasksRunning(&ecs.DescribeTasksInput{
			Cluster: shellTask.ClusterArn,
			Tasks:   []*string{shellTask.TaskArn},
		})
		checkErr(err)
		Spinner.Stop()
		err = app.ConnectToTask(shellTask, aws.String("entrypoint.sh psql"))
		checkErr(err)
	},
}

// dbLoadCmd represents the db load command
var dbLoadCmd = &cobra.Command{
	Use:   "load",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("load called")
	},
}

func init() {
	rootCmd.AddCommand(dbCmd)

	dbCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "App name (required)")
	dbCmd.MarkPersistentFlagRequired("app-name")
	dbCmd.AddCommand(dbShellCmd)

	dbCmd.AddCommand(dbLoadCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dbCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dbCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
