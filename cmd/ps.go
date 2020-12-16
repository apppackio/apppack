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

	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/dustin/go-humanize"
	"github.com/lincolnloop/apppack/app"
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
	Use:   "ps",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
			tag, err := getTag(t.Tags, "paaws:processType")
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
				buildNumber, err := getTag(t.Tags, "paaws:buildNumber")
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
	Use:   "resize",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		processType := args[0]
		a, err := app.Init(AppName)
		checkErr(err)
		err = a.ResizeProcess(processType, scaleCPU, scaleMemory)
		checkErr(err)
		printSuccess(fmt.Sprintf("resizing %s", processType))
	},
}

// psResizeCmd represents the resize command
var psScaleCmd = &cobra.Command{
	Use:   "scale",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		processType := args[0]
		processCount, err := strconv.Atoi(args[1])
		checkErr(err)
		a, err := app.Init(AppName)
		checkErr(err)
		err = a.ScaleProcess(processType, processCount)
		checkErr(err)
		printSuccess(fmt.Sprintf("scaling %s", processType))
	},
}

var scaleCPU int
var scaleMemory int

func init() {
	rootCmd.AddCommand(psCmd)
	psCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "App name (required)")
	psCmd.MarkPersistentFlagRequired("app-name")

	psCmd.AddCommand(psResizeCmd)
	psResizeCmd.Flags().IntVarP(&scaleCPU, "cpu", "c", 1024, "CPU shares where 1024 is 1 full CPU")
	psResizeCmd.Flags().IntVarP(&scaleMemory, "memory", "m", 2048, "Memory in megabytes")

	psCmd.AddCommand(psScaleCmd)
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// psCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// psCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
