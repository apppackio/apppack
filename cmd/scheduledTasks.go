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
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apppackio/apppack/app"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

func printTasks(tasks []*app.ScheduledTask) {
	if len(tasks) == 0 {
		fmt.Printf("%s", aurora.Yellow(fmt.Sprintf("no scheduled tasks defined")))
		return
	} else {
		fmt.Printf("%s\n", aurora.Faint("Min\tHr\tDayMon\tMon\tDayWk\tYr"))
	}
	for _, task := range tasks {
		fmt.Printf("%s\t%s\n", aurora.Faint(strings.Join(strings.Split(task.Schedule, " "), "\t")), task.Command)
	}
}

// scheduledTasksCmd represents the scheduledTasks command
var scheduledTasksCmd = &cobra.Command{
	Use:                   "scheduled-tasks",
	Short:                 "list scheduled tasks",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		tasks, err := a.ScheduledTasks()
		Spinner.Stop()
		checkErr(err)
		printTasks(tasks)
	},
}

var schedule string

// scheduledTasksCreateCmd represents the create command
var scheduledTasksCreateCmd = &cobra.Command{
	Use:   "create --schedule \"<min> <hr> <day-mon> <mon> <day-wk> <yr>\" \"<command>\"",
	Args:  cobra.ExactArgs(1),
	Short: "schedule a task",
	Long: `Schedule a command to run on a recurring schedule in the future.

Be sure to wrap your command and schedule in quotes to ensure they are read as a single arguement. The schedule flag should use the AWS cron-like format as described at https://docs.aws.amazon.com/eventbridge/latest/userguide/scheduled-events.html#cron-expressions"`,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		if len(strings.Split(schedule, " ")) != 6 {
			checkErr(fmt.Errorf("schedule string should contain 6 space separated values\nhttps://docs.aws.amazon.com/eventbridge/latest/userguide/scheduled-events.html#cron-expressions"))
		}
		command := strings.Join(args, " ")
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		tasks, err := a.CreateScheduledTask(schedule, command)
		Spinner.Stop()
		checkErr(err)
		printSuccess("task created")
		printTasks(tasks)
	},
}

// scheduledTasksDeleteCmd represents the delete command
var scheduledTasksDeleteCmd = &cobra.Command{
	Use:   "delete [<index>]",
	Args:  cobra.MaximumNArgs(1),
	Short: "delete an existing scheduled task",
	Long: `Delete the scheduled task at the provided index.

If no index is provided, an interactive prompt will be provided to choose the task to delete.`,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		var idx int
		var task *app.ScheduledTask
		if len(args) > 0 {
			idx, err = strconv.Atoi(args[0])
			checkErr(err)
			idx--
		} else {
			tasks, err := a.ScheduledTasks()
			checkErr(err)
			taskList := []string{}
			for _, t := range tasks {
				taskList = append(taskList, fmt.Sprintf("%s %s", t.Schedule, t.Command))
			}
			questions := []*survey.Question{{
				Name: "task",
				Prompt: &survey.Select{
					Message:       "Scheduled task to delete:",
					Options:       taskList,
					FilterMessage: "",
				},
			}}
			answers := make(map[string]int)
			Spinner.Stop()
			if err := survey.Ask(questions, &answers); err != nil {
				checkErr(err)
			}
			idx = answers["task"]
		}
		task, err = a.DeleteScheduledTask(idx)
		checkErr(err)
		printSuccess("scheduled task deleted:")
		fmt.Printf("  %s %s\n", aurora.Faint(task.Schedule), task.Command)
	},
}

func init() {
	rootCmd.AddCommand(scheduledTasksCmd)
	scheduledTasksCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	scheduledTasksCmd.MarkPersistentFlagRequired("app-name")

	scheduledTasksCmd.AddCommand(scheduledTasksCreateCmd)
	scheduledTasksCreateCmd.Flags().StringVarP(&schedule, "schedule", "s", "", "cron-like schedule. See https://docs.aws.amazon.com/eventbridge/latest/userguide/scheduled-events.html#cron-expressions")

	scheduledTasksCmd.AddCommand(scheduledTasksDeleteCmd)
}
