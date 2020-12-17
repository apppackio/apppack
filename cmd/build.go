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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/dustin/go-humanize"
	"github.com/lincolnloop/apppack/app"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

const indentStr = "    "

func indent(text, indent string) string {
	if len(text) == 0 {
		return indent
	}
	if text[len(text)-1:] == "\n" {
		result := ""
		for _, j := range strings.Split(text[:len(text)-1], "\n") {
			result += indent + j + "\n"
		}
		return result
	}
	result := ""
	for _, j := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		result += indent + j + "\n"
	}
	return result[:len(result)-1]
}

func printBuild(build *codebuild.Build, commitLog []byte) error {
	icon := map[string]aurora.Value{
		"IN_PROGRESS": aurora.Faint(aurora.Blue("ℹ")),
		"SUCCEEDED":   aurora.Green("✔"),
		"FAILED":      aurora.Red("✖"),
	}
	fmt.Print(aurora.Faint("==="), aurora.White(fmt.Sprintf("%d", *build.BuildNumber)))
	if build.BuildStatus == aws.String("IN_PROGRESS") {
		fmt.Print(" in progress")
	}
	fmt.Print(" ", aurora.Blue(*build.SourceVersion), icon[*build.BuildStatus])
	fmt.Println()
	if build.EndTime != nil {
		fmt.Println(aurora.Faint(fmt.Sprintf("%s%s ~ %s", indentStr, build.EndTime.Local().Format(timeFmt), humanize.Time(*build.EndTime))))
	} else {
		fmt.Println(aurora.Faint(fmt.Sprintf("%sstarted %s ~ %s", indentStr, build.StartTime.Local().Format(timeFmt), humanize.Time(*build.StartTime))))
	}
	fmt.Println(indent(fmt.Sprintf("%s", commitLog), indentStr))
	return nil
}

func watchBuild(a *app.App, build *codebuild.Build) error {
	var lastPhase string
	var lastUpdate time.Time
	var status string
	startSpinner()
	retries := 10
	for {
		// retry a few times for the initial status to show up
		// once it does, stop retrying
		deployStatus, err := a.BuildStatus(build)
		if err != nil {
			if retries > 0 {
				retries--
				time.Sleep(3 * time.Second)
				continue
			} else {
				return err
			}
		}
		retries = 0
		lastUpdate = time.Unix(deployStatus.LastUpdate, 0)
		if lastPhase != deployStatus.Phase {
			Spinner.Stop()
			if deployStatus.Phase == "running" {
				status = "Deploy phase complete"
			} else {
				status = fmt.Sprintf("%s phase started", strings.Title(deployStatus.Phase))
			}
			fmt.Printf("%s\t%s\n", aurora.Yellow(status), aurora.Faint(lastUpdate.Local().Format(timeFmt)))
			lastPhase = deployStatus.Phase
			startSpinner()
			// sleep for a second so the spinner doesn't show "X running for now"
			time.Sleep(500 * time.Millisecond)
		}
		Spinner.Suffix = fmt.Sprintf(" %s running for %s", deployStatus.Phase, strings.Replace(humanize.Time(lastUpdate), " ago", "", 1))
		if deployStatus.Phase == "running" {
			Spinner.Stop()
			printSuccess(fmt.Sprintf("build #%d deployed successfully", *build.BuildNumber))
			break
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Work with AppPack builds",
	Long:  `Use to view, list, and trigger code builds.`,
}

// buildStartCmd represents the start command
var buildStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new build from the latest commit on the branch defined in AppPack",
	Long:  `Start a new build from the latest commit on the branch defined in AppPack`,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		build, err := a.StartBuild()
		checkErr(err)
		Spinner.Stop()
		printSuccess("build started")
		err = printBuild(build, []byte{})
		checkErr(err)
		if watchBuildFlag {
			err = watchBuild(a, build)
			checkErr(err)
		}
	},
}

// buildWaitCmd represents the start command
var buildWaitCmd = &cobra.Command{
	Use:   "wait",
	Short: "wait for the most recent build to be deployed",
	Long:  `Bait for the most recent build to be deployed`,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		build, err := a.LastBuild()
		checkErr(err)
		Spinner.Stop()
		commitLog, _ := a.GetBuildArtifact(build, "commit.txt")
		err = printBuild(build, commitLog)
		checkErr(err)
		err = watchBuild(a, build)
		checkErr(err)
	},
}

// buildListCmd represents the list command
var buildListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent builds",
	Long:  `List recent builds`,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		builds, err := a.ListBuilds()
		checkErr(err)
		Spinner.Stop()
		for _, build := range builds {
			commitLog, _ := a.GetBuildArtifact(build, "commit.txt")
			err = printBuild(build, commitLog)
			checkErr(err)
		}
	},
}

var watchBuildFlag bool

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "App name (required)")
	buildCmd.MarkPersistentFlagRequired("app-name")

	buildCmd.AddCommand(buildStartCmd)
	buildStartCmd.Flags().BoolVarP(&watchBuildFlag, "wait", "w", false, "wait for build to complete")
	buildCmd.AddCommand(buildListCmd)

	buildCmd.AddCommand(buildWaitCmd)
}
