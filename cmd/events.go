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
	"strings"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

func printEvent(event *ecs.ServiceEvent) {
	fmt.Println(aurora.Faint(event.CreatedAt.Local().Format("Jan 02, 2006 15:04:05 MST")), *event.Message)
}

// eventsCmd represents the events command
var eventsCmd = &cobra.Command{
	Use:     "events <service>",
	Short:   "Show recent events for the given service",
	Example: "apppack -a my-app events web",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		events, err := a.GetECSEvents(args[0])
		checkErr(err)
		waitForSteadyState := cmd.Flag("wait-for-steady").Value.String() == "true"
		Spinner.Stop()
		fmt.Println("⏳", aurora.Blue(fmt.Sprintf("waiting for `%s` service to reach a steady state...", args[0])))
		for _, event := range events {
			// when waiting for steady, only show the last minute of events to start
			if !waitForSteadyState || event.CreatedAt.After(time.Now().Add(-time.Minute)) {
				printEvent(event)
			}
		}
		if !waitForSteadyState {
			return
		}
		startSpinner()
		// if no events, wait for some to come in
		if len(events) == 0 {
			for {
				events, err = a.GetECSEvents(args[0])
				checkErr(err)
				if len(events) > 0 {
					break
				}
				time.Sleep(time.Second * 5)
			}
		}
		cursor := *events[len(events)-1].Id
		startEventTime := events[0].CreatedAt
		for {
			events, err := a.GetECSEvents(args[0])
			checkErr(err)
			lastEvent := events[len(events)-1]
			// no new events, sleep and check again
			if *lastEvent.Id == cursor {
				time.Sleep(time.Second * 5)
				continue
			}
			Spinner.Stop()
			display := false
			// loop through events, only printing new ones and break on steady state
			for _, event := range events {
				if display {
					printEvent(event)
					if strings.Contains(*event.Message, "has reached a steady state") && event.CreatedAt.After(*startEventTime) {
						return
					}
				} else if *event.Id == cursor {
					display = true
				}
			}
			startSpinner()
			cursor = *lastEvent.Id
			time.Sleep(time.Second * 5)
		}
	},
}

func init() {
	rootCmd.AddCommand(eventsCmd)
	eventsCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	eventsCmd.MarkPersistentFlagRequired("app-name")
	eventsCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	eventsCmd.Flags().BoolP("wait-for-steady", "w", false, "wait for service to reach a steady state")
}
