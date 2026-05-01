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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/ui"
	"github.com/charmbracelet/huh"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

// scheduledTaskJSON is a JSON-serializable representation of a ScheduledTask.
// Using an explicit wrapper decouples the JSON contract from internal struct tags.
type scheduledTaskJSON struct {
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

func toScheduledTaskJSON(t *app.ScheduledTask) scheduledTaskJSON {
	return scheduledTaskJSON{
		Schedule: t.Schedule,
		Command:  t.Command,
	}
}

func printTasks(tasks []*app.ScheduledTask) {
	if len(tasks) == 0 {
		fmt.Printf("%s\n", aurora.Yellow("no scheduled tasks defined"))

		return
	}

	fmt.Printf("%s\n", aurora.Faint("Min\tHr\tDayMon\tMon\tDayWk\tYr"))

	for _, task := range tasks {
		fmt.Printf("%s\t%s\n", aurora.Faint(strings.Join(strings.Split(task.Schedule, " "), "\t")), task.Command)
	}
}

// scheduledTasksCmd represents the scheduledTasks command
var scheduledTasksCmd = &cobra.Command{
	Use:                   "scheduled-tasks",
	Short:                 "list scheduled tasks",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(0),
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		if a.IsReviewApp() {
			checkErr(errors.New("review apps do not currently support scheduled tasks"))
		}
		tasks, err := a.ScheduledTasks()
		ui.Spinner.Stop()
		checkErr(err)

		if AsJSON {
			wrapped := make([]scheduledTaskJSON, 0, len(tasks))
			for _, t := range tasks {
				wrapped = append(wrapped, toScheduledTaskJSON(t))
			}
			checkErr(printJSON(wrapped))

			return
		}

		printTasks(tasks)
	},
}

var schedule string

// scheduledTasksCreateCmd represents the create command
var scheduledTasksCreateCmd = &cobra.Command{
	Use:   "create --schedule \"<min> <hr> <day-mon> <mon> <day-wk> <yr>\" \"<command>\"",
	Args:  cobra.ExactArgs(1),
	Short: "schedule a task",
	Example: `apppack -a my-app scheduled-tasks create --schedule "0 0 * * ? *" "your-command --args"  # run daily at midnight UTC
apppack -a my-app scheduled-tasks create --schedule "0/10 * * * ? *" "your-command --args"  # run every 10 minutes`,
	Long: `Schedule a command to run on a recurring schedule in the future.

Be sure to wrap your command and schedule in quotes to ensure they are read as a single argument and note that the timezone is always UTC. The schedule flag should use the AWS cron-like format as described at https://docs.aws.amazon.com/eventbridge/latest/userguide/scheduled-events.html#cron-expressions`,
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, args []string) {
		if len(strings.Split(schedule, " ")) != 6 {
			checkErr(errors.New("schedule string should contain 6 space separated values\nhttps://docs.aws.amazon.com/eventbridge/latest/userguide/scheduled-events.html#cron-expressions"))
		}
		command := strings.Join(args, " ")
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		if a.IsReviewApp() {
			checkErr(errors.New("review apps do not currently support scheduled tasks"))
		}
		tasks, err := a.CreateScheduledTask(schedule, command)
		ui.Spinner.Stop()
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
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		if a.IsReviewApp() {
			checkErr(errors.New("review apps do not currently support scheduled tasks"))
		}
		var idx int
		var task *app.ScheduledTask
		if len(args) > 0 {
			idx, err = strconv.Atoi(args[0])
			checkErr(err)
			idx--
		} else {
			tasks, err := a.ScheduledTasks()
			checkErr(err)
			if len(tasks) == 0 {
				checkErr(errors.New("no scheduled tasks to delete"))

				return
			}
			options := make([]huh.Option[int], len(tasks))
			for i, t := range tasks {
				options[i] = huh.NewOption(fmt.Sprintf("%s %s", t.Schedule, t.Command), i)
			}
			ui.Spinner.Stop()
			form, idxPtr := ScheduledTaskDeleteForm(options)
			checkErr(form.Run())
			idx = *idxPtr
		}
		task, err = a.DeleteScheduledTask(idx)
		checkErr(err)
		printSuccess("scheduled task deleted:")
		fmt.Printf("  %s %s\n", aurora.Faint(task.Schedule), task.Command)
	},
}

// ScheduledTaskDeleteForm builds the interactive form for selecting a task to delete.
// Returns the form and a pointer to the selected index.
func ScheduledTaskDeleteForm(options []huh.Option[int]) (*huh.Form, *int) {
	var idx int

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Scheduled task to delete:").
				Options(options...).
				Value(&idx),
		),
	)

	return form, &idx
}

func init() {
	rootCmd.AddCommand(scheduledTasksCmd)
	scheduledTasksCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	scheduledTasksCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")

	scheduledTasksCmd.AddCommand(scheduledTasksCreateCmd)
	scheduledTasksCreateCmd.Flags().StringVarP(&schedule, "schedule", "s", "", "cron-like schedule. See https://docs.aws.amazon.com/eventbridge/latest/userguide/scheduled-events.html#cron-expressions")

	scheduledTasksCmd.AddCommand(scheduledTasksDeleteCmd)
}
