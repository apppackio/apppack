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
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// WaitForTaskRunning waits for a task to be running or complete
func WaitForTaskRunning(a *app.App, task *ecs.Task) error {
	ecsSvc := ecs.New(a.Session)

	status := ""
	for status != "RUNNING" {
		time.Sleep(2 * time.Second)

		out, err := ecsSvc.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: &a.Settings.Cluster.ARN,
			Tasks:   []*string{task.TaskArn},
		})
		if err != nil {
			return err
		}

		status = *out.Tasks[0].LastStatus
		if status == "DEACTIVATING" || status == "STOPPING" || status == "DEPROVISIONING" || status == "STOPPED" {
			return fmt.Errorf("task is not running -- last status: %s", status)
		}

		caser := cases.Title(language.English)
		ui.Spinner.Suffix = " ECS task status: " + caser.String(strings.ToLower(status))
	}

	ui.Spinner.Suffix = ""

	return nil
}

func StartInteractiveShell(a *app.App, taskFamily, shellCmd *string, taskCommandPrefix []string, taskOverride *ecs.TaskOverride) {
	task, err := a.StartTask(
		taskFamily,
		append(taskCommandPrefix, app.ShellBackgroundCommand...),
		taskOverride,
		false,
	)
	checkErr(err)
	ui.Spinner.Stop()
	fmt.Println(aurora.Faint("starting " + *task.TaskArn))
	ui.StartSpinner()
	checkErr(WaitForTaskRunning(a, task))
	ui.Spinner.Stop()
	fmt.Println(aurora.Faint("waiting for SSM Agent to startup"))
	ui.StartSpinner()

	ecsSession, err := a.CreateEcsSession(task, *shellCmd)
	checkErr(err)
	ui.Spinner.Stop()

	err = a.ConnectToEcsSession(ecsSession)
	checkErr(err)
}

var (
	shellCPU  float64
	shellMem  string
	shellRoot bool
	shellLive bool
)

func humanToECSSizeConfiguration(cpu float64, memory string) (*app.ECSSizeConfiguration, error) {
	var memoryInMB int

	var memoryInGB int

	var err error

	fargateCPU := int(math.RoundToEven(cpu * 1024))

	if strings.HasSuffix(memory, "G") {
		memoryInGB, err = strconv.Atoi(memory[:len(memory)-1])
		if err != nil {
			return nil, err
		}

		return &app.ECSSizeConfiguration{CPU: fargateCPU, Memory: memoryInGB * 1024}, nil
	} else if strings.HasSuffix(memory, "GB") {
		memoryInGB, err = strconv.Atoi(memory[:len(memory)-2])
		if err != nil {
			return nil, err
		}

		return &app.ECSSizeConfiguration{CPU: fargateCPU, Memory: memoryInGB * 1024}, nil
	} else if strings.HasSuffix(memory, "M") {
		memoryInMB, err = strconv.Atoi(memory[:len(memory)-1])
		if err != nil {
			return nil, err
		}

		return &app.ECSSizeConfiguration{CPU: fargateCPU, Memory: memoryInMB}, nil
	} else if strings.HasSuffix(memory, "MB") {
		memoryInMB, err = strconv.Atoi(memory[:len(memory)-2])
		if err != nil {
			return nil, err
		}

		return &app.ECSSizeConfiguration{CPU: fargateCPU, Memory: memoryInMB}, nil
	}

	return nil, errors.New("unexpected memory format -- it must end in 'M' (for MB) or 'G' (for GB)")
}

func interactiveCmd(a *app.App, cmd string) {
	taskFamily, buildSystem, err := a.ShellTaskFamily()
	checkErr(err)
	size, err := humanToECSSizeConfiguration(shellCPU, shellMem)
	checkErr(err)
	checkErr(a.ValidateECSTaskSize(*size))

	isBuildpack := *buildSystem == "buildpacks" || *buildSystem == ""
	exec := cmd

	if isBuildpack && !shellRoot {
		exec = fmt.Sprintf("su --preserve-environment --pty --command 'export HOME=$(getent passwd heroku | cut -d: -f6); exec /cnb/lifecycle/launcher %s' heroku", cmd)
	} else if !isBuildpack && shellRoot {
		checkErr(errors.New("--root is only supported on the buildpack build system"))
	}

	if shellLive {
		tasks, err := a.DescribeTasks()
		checkErr(err)

		var taskList []string

		for _, t := range tasks {
			tag, err := getTag(t.Tags, "apppack:processType")
			if err != nil {
				continue
			}

			arnParts := strings.Split(*t.TaskArn, "/")
			taskList = append(taskList, fmt.Sprintf("%s: %s", *tag, arnParts[len(arnParts)-1]))
		}

		answers := make(map[string]interface{})
		questions := []*survey.Question{
			{
				Name: "task",
				Prompt: &survey.Select{
					Message: "Select task to connect to",
					Options: taskList,
				},
			},
		}

		ui.Spinner.Stop()

		if err := survey.Ask(questions, &answers); err != nil {
			checkErr(err)
		}

		ui.StartSpinner()

		ecsSession, err := a.CreateEcsSession(
			tasks[answers["task"].(survey.OptionAnswer).Index],
			exec,
		)
		checkErr(err)
		ui.Spinner.Stop()

		err = a.ConnectToEcsSession(ecsSession)
		checkErr(err)
	}

	var taskCommandPrefix []string
	if !isBuildpack {
		// buildpacks already wrap commands in `bash -c`
		taskCommandPrefix = []string{"/bin/sh", "-c"}
	}

	StartInteractiveShell(a, taskFamily, &exec, taskCommandPrefix, &ecs.TaskOverride{
		Cpu:    aws.String(strconv.Itoa(size.CPU)),
		Memory: aws.String(strconv.Itoa(size.Memory)),
	})
}

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:                   "shell",
	Short:                 "open an interactive shell in the remote environment",
	Long:                  `Open an interactive shell in the remote environment`,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		interactiveCmd(a, "bash -l")
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	shellCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	shellCmd.PersistentFlags().BoolVarP(&shellRoot, "root", "r", false, "open shell as root user")
	shellCmd.PersistentFlags().BoolVarP(&shellLive, "live", "l", false, "connect to a live process")
	shellCmd.Flags().Float64Var(&shellCPU, "cpu", 0.5, "CPU cores available for task")
	shellCmd.Flags().StringVar(&shellMem, "memory", "1G", "memory (e.g. '2G', '512M') available for task")
	shellCmd.MarkPersistentFlagRequired("app-name")
}
