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
	"sort"
	"strconv"
	"strings"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

func getTag(tags []*ecs.Tag, key string) (*string, error) {
	for _, tag := range tags {
		if *tag.Key == key {
			return tag.Value, nil
		}
	}
	return nil, fmt.Errorf("tag %s not found", key)
}

// psCmd represents the ps command
var psCmd = &cobra.Command{
	Use:                   "ps",
	Short:                 "show running processes",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		tasks, err := a.DescribeTasks()
		Spinner.Stop()
		checkErr(err)
		// group tasks by process type
		grouped := map[string][]*ecs.Task{}
		for _, t := range tasks {
			tag, err := getTag(t.Tags, "apppack:processType")
			if err != nil {
				continue
			}
			grouped[*tag] = append(grouped[*tag], t)
		}
		// sort process types
		keys := make([]string, 0, len(grouped))
		for k := range grouped {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// iterate over process types/tasks
		for _, proc := range keys {
			status := a.DeployStatus.Processes[proc]
			fmt.Printf("%s %s %s ", aurora.Faint("==="), aurora.Green(proc), aurora.White(status.Command))
			if status.MinProcesses == status.MaxProcesses {
				fmt.Printf("(%s)\n", aurora.Yellow(fmt.Sprintf("%d", status.MinProcesses)))
			} else {
				fmt.Printf("(%s)\n", aurora.Yellow(fmt.Sprintf("%d - %d", status.MinProcesses, status.MaxProcesses)))
			}
			tasks := grouped[proc]
			sort.SliceStable(tasks, func(i, j int) bool {
				if tasks[i].StartedAt == nil {
					return false
				} else if tasks[j].StartedAt == nil {
					return true
				}
				return tasks[i].StartedAt.After(*tasks[j].StartedAt)
			})
			for i, t := range tasks {
				name := fmt.Sprintf("%s.%d", proc, i)
				cpu, err := strconv.ParseFloat(*t.Cpu, 32)
				checkErr(err)
				cpu = cpu / 1024.0
				buildNumber, err := getTag(t.Tags, "apppack:buildNumber")
				checkErr(err)
				fmt.Printf("%s: %s (%s) %s %s\n", name, strings.ToLower(*t.LastStatus), aurora.Bold(aurora.Cyan(fmt.Sprintf("%.2fcpu/%smem", cpu, *t.Memory))), aurora.Yellow(fmt.Sprintf("build #%s", *buildNumber)), aurora.Faint(fmt.Sprintf("%s (~ %s)", t.StartedAt.Local().Format("Jan 02, 2006 15:04:05 MST"), humanize.Time(*t.StartedAt))))
				indent := strings.Repeat(" ", len(name)+2)
				fmt.Printf("%s%s\n", indent, aurora.Faint(*t.TaskArn))
			}

		}
	},
}

// psResizeCmd represents the resize command
var psResizeCmd = &cobra.Command{
	Use:                   "resize <process_type>",
	Short:                 "resize (CPU/memory) the process for a given type",
	DisableFlagsInUseLine: true,
	Example:               "apppack resize web --cpu 2048 --memory 4096",
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		processType := args[0]
		a, err := app.Init(AppName)
		checkErr(err)
		err = a.ResizeProcess(processType, scaleCPU, scaleMemory)
		checkErr(err)
		printSuccess(fmt.Sprintf("resizing %s", processType))
	},
}

// psScaleCmd represents the scale command
var psScaleCmd = &cobra.Command{
	Use:   "scale <process_type> <process_count>",
	Short: "scale the number of processes that run for a specific process type",
	Long: `Scale the number of processes that run for a specific process type.

` + "`<process_count>`" + ` can either be a single number, e.g. 2 or a range, e.g. 1-5. When
a range is provided, the process will autoscale within that range based on CPU usage.`,
	Example: `apppack ps scale web 3  # run 3 web processes
apppack ps scale worker 1-4  # autoscale worker service from 1 to 4 processes`,
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		processType := args[0]
		var minProcesses int
		var maxProcesses int
		var out string
		minMaxProcs := strings.Split(args[1], "-")
		minProcesses, err := strconv.Atoi(minMaxProcs[0])
		checkErr(err)
		if len(minMaxProcs) > 1 {
			maxProcesses, err = strconv.Atoi(minMaxProcs[1])
			out = fmt.Sprintf("%s will autoscale from %d to %d processes", processType, minProcesses, maxProcesses)
		} else {
			maxProcesses = minProcesses
			out = fmt.Sprintf("%s will scale to %d processes", processType, minProcesses)
		}
		a, err := app.Init(AppName)
		checkErr(err)
		startSpinner()
		err = a.ScaleProcess(processType, minProcesses, maxProcesses)
		checkErr(err)
		Spinner.Stop()
		printSuccess(out)
	},
}

var scaleCPU int
var scaleMemory int

func init() {
	rootCmd.AddCommand(psCmd)
	psCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	psCmd.MarkPersistentFlagRequired("app-name")

	psCmd.AddCommand(psResizeCmd)
	psResizeCmd.Flags().IntVarP(&scaleCPU, "cpu", "c", 1024, "CPU shares where 1024 is 1 full CPU")
	psResizeCmd.Flags().IntVarP(&scaleMemory, "memory", "m", 2048, "memory in megabytes")

	psCmd.AddCommand(psScaleCmd)
}
