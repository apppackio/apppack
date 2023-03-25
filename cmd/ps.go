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
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
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

func printTask(t *ecs.Task, count *int) {
	tag, err := getTag(t.Tags, "apppack:processType")
	checkErr(err)
	name := *tag
	if count != nil {
		name = fmt.Sprintf("%s.%d", name, count)
	}

	cpu, err := strconv.ParseFloat(*t.Cpu, 32)
	checkErr(err)
	cpu = cpu / 1024.0
	buildNumber, err := getTag(t.Tags, "apppack:buildNumber")
	checkErr(err)
	var startText string
	if t.StartedAt == nil {
		startText = ""
	} else {
		startText = fmt.Sprintf("%s (~ %s)", t.StartedAt.Local().Format("Jan 02, 2006 15:04:05 MST"), humanize.Time(*t.StartedAt))
	}
	fmt.Printf("%s: %s (%s) %s %s\n", name, strings.ToLower(*t.LastStatus), aurora.Bold(aurora.Cyan(fmt.Sprintf("%.2fcpu/%smem", cpu, *t.Memory))), aurora.Yellow(fmt.Sprintf("build #%s", *buildNumber)), aurora.Faint(startText))
	indent := strings.Repeat(" ", len(name)+2)
	if *tag == "shell" {
		fmt.Printf("%s%s\n", indent, aurora.Faint(fmt.Sprintf("started by: %s", *t.StartedBy)))
	}
	fmt.Printf("%s%s\n", indent, aurora.Faint(*t.TaskArn))
}

// psCmd represents the ps command
var psCmd = &cobra.Command{
	Use:                   "ps",
	Short:                 "show running processes",
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		if a.Pipeline && !a.IsReviewApp() {
			checkErr(fmt.Errorf("pipelines don't directly run processes"))
		}
		tasks, err := a.DescribeTasks()
		ui.Spinner.Stop()
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
		err = a.LoadDeployStatus()
		checkErr(err)
		var extraProcs []*ecs.Task
		// iterate over process types/tasks
		for _, proc := range keys {
			status, err := a.DeployStatus.FindProcess(proc)
			if err != nil {
				logrus.WithFields(logrus.Fields{"err": err, "service": proc}).Debug("service not in deploy status")
			}
			tasks := grouped[proc]
			if status == nil {
				extraProcs = append(extraProcs, tasks...)
				continue
			}

			fmt.Printf("%s %s %s ", aurora.Faint("==="), aurora.Green(proc), aurora.White(status.Command))
			if status.MinProcesses == status.MaxProcesses {
				fmt.Printf("(%s)\n", aurora.Yellow(fmt.Sprintf("%d", status.MinProcesses)))
			} else {
				fmt.Printf("(%s)\n", aurora.Yellow(fmt.Sprintf("%d - %d", status.MinProcesses, status.MaxProcesses)))
			}
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
				var startText string
				if t.StartedAt == nil {
					startText = ""
				} else {
					startText = fmt.Sprintf("%s (~ %s)", t.StartedAt.Local().Format("Jan 02, 2006 15:04:05 MST"), humanize.Time(*t.StartedAt))
				}
				fmt.Printf("%s: %s (%s) %s %s\n", name, strings.ToLower(*t.LastStatus), aurora.Bold(aurora.Cyan(fmt.Sprintf("%.2fcpu/%smem", cpu, *t.Memory))), aurora.Yellow(fmt.Sprintf("build #%s", *buildNumber)), aurora.Faint(startText))
				indent := strings.Repeat(" ", len(name)+2)
				fmt.Printf("%s%s\n", indent, aurora.Faint(*t.TaskArn))
			}

		}
		if len(extraProcs) > 0 {
			fmt.Printf("\n")
		}
		for _, t := range extraProcs {
			printTask(t, nil)
		}
	},
}

// psResizeCmd represents the resize command
var psResizeCmd = &cobra.Command{
	Use:                   "resize <process_type>",
	Short:                 "resize (CPU/memory) the process for a given type",
	DisableFlagsInUseLine: true,
	Example:               "apppack -a my-app ps resize web --cpu 2 --memory 4G",
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		processType := args[0]
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		size, err := humanToECSSizeConfiguration(scaleCPU, scaleMemory)
		checkErr(err)
		checkErr(a.ValidateECSTaskSize(*size))
		err = a.ResizeProcess(processType, size.CPU, size.Memory)
		checkErr(err)
		if a.Pipeline {
			printSuccess(fmt.Sprintf("set default size for %s processes on review apps", processType))
		} else {
			printSuccess(fmt.Sprintf("resizing %s", processType))
		}
	},
}

// psScaleCmd represents the scale command
var psScaleCmd = &cobra.Command{
	Use:   "scale <process_type> <process_count>",
	Short: "scale the number of processes that run for a specific process type",
	Long: `Scale the number of processes that run for a specific process type.

` + "`<process_count>`" + ` can either be a single number, e.g. 2 or a range, e.g. 1-5. When
a range is provided, the process will autoscale within that range based on CPU usage.`,
	Example: `apppack -a my-app ps scale web 3  # run 3 web processes
apppack -a my-app ps scale worker 1-4  # autoscale worker service from 1 to 4 processes`,
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
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		if a.IsReviewApp() {
			checkErr(fmt.Errorf("scaling is not supported for review apps"))
		}
		if len(minMaxProcs) > 1 {
			maxProcesses, err = strconv.Atoi(minMaxProcs[1])
			checkErr(err)
			out = fmt.Sprintf("%s will autoscale from %d to %d processes", processType, minProcesses, maxProcesses)
		} else {
			maxProcesses = minProcesses
			out = fmt.Sprintf("%s will scale to %d processes", processType, minProcesses)
		}
		ui.StartSpinner()
		err = a.ScaleProcess(processType, minProcesses, maxProcesses)
		checkErr(err)
		ui.Spinner.Stop()
		printSuccess(out)
	},
}

// execCmd represents the exec command
var psExecCmd = &cobra.Command{
	Use:   "exec -- <command>",
	Args:  cobra.MinimumNArgs(1),
	Short: "run an interactive command in the remote environment",
	Long: `Run an interactive command in the remote environment

Requires installation of Amazon's SSM Session Manager. https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html`,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		interactiveCmd(a, strings.Join(args, " "))
	},
}

var scaleCPU float64
var scaleMemory string

func init() {
	rootCmd.AddCommand(psCmd)
	psCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	psCmd.MarkPersistentFlagRequired("app-name")
	psCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")

	psCmd.AddCommand(psResizeCmd)
	psResizeCmd.Flags().Float64Var(&scaleCPU, "cpu", 0.5, "CPU cores available for process")
	psResizeCmd.Flags().StringVar(&scaleMemory, "memory", "1G", "memory (e.g. '2G', '512M') available for process")

	psCmd.AddCommand(psScaleCmd)
	psCmd.AddCommand(psExecCmd)
	psExecCmd.PersistentFlags().BoolVarP(&shellRoot, "root", "r", false, "open shell as root user")
	psExecCmd.PersistentFlags().BoolVarP(&shellLive, "live", "l", false, "connect to a live process")
	psExecCmd.Flags().Float64Var(&shellCpu, "cpu", 0.5, "CPU cores available for task")
	psExecCmd.Flags().StringVar(&shellMem, "memory", "1G", "memory (e.g. '2G', '512M') available for task")
}
