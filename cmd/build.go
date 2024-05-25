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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	indentStr = "    "
	Started   = "started"
)

func indent(text, indent string) string {
	if text == "" {
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

func printBuild(buildStatus *app.BuildStatus) {
	icon := map[string]aurora.Value{
		app.PhaseInProgress: aurora.Faint(aurora.Blue("ℹ")),
		app.PhaseSuccess:    aurora.Green("✔"),
		app.PhaseFailed:     aurora.Red("✖"),
	}
	ui.PrintHeader(fmt.Sprintf("%d", buildStatus.BuildNumber))
	currentPhase := buildStatus.CurrentPhase()
	if currentPhase != nil {
		fmt.Print(" in progress")
	}
	fmt.Print(" ", aurora.Blue(buildStatus.Commit))
	fmt.Printf("\n%s", indentStr)
	finalPhase, _ := buildStatus.FinalPhase()
	for _, p := range buildStatus.NamedPhases() {
		if p.Phase.State == "" {
			continue
		}
		fmt.Printf("%s %s", icon[p.Phase.State], p.Name)
		if (currentPhase != nil && p.Name == currentPhase.Name) || (finalPhase != nil && p.Name == finalPhase.Name) {
			fmt.Println()

			break
		}
		fmt.Print(aurora.Faint("  |  ").String())
	}
	if finalPhase != nil {
		fmt.Println(aurora.Faint(fmt.Sprintf("%s%s ~ %s", indentStr, finalPhase.Phase.EndTime().Local().Format(timeFmt), humanize.Time(finalPhase.Phase.EndTime()))))
	} else {
		fmt.Println(aurora.Faint(fmt.Sprintf("%sstarted %s ~ %s", indentStr, buildStatus.Build.StartTime().Local().Format(timeFmt), humanize.Time(buildStatus.Build.StartTime()))))
	}
}

func printCommitLog(sess *session.Session, buildStatus *app.BuildStatus) {
	commitLog, err := buildStatus.GetCommitLog(sess)
	if err == nil {
		fmt.Println(indent(*commitLog, indentStr))
	} else {
		fmt.Print(indentStr)
		printWarning(fmt.Sprintf("unable to read commit data for build #%d", buildStatus.BuildNumber))
		fmt.Println()
	}
}

func pollBuildStatus(a *app.App, buildNumber, retries int) (*app.BuildStatus, error) {
	// retry a few times for the initial status to show up
	// once it does, stop retrying
	buildStatus, err := a.GetBuildStatus(buildNumber)
	if err != nil {
		if retries > 0 {
			time.Sleep(3 * time.Second)
			return pollBuildStatus(a, buildNumber, retries-1)
		}
		return nil, err
	}
	return buildStatus, nil
}

func watchBuild(a *app.App, buildStatus *app.BuildStatus) error {
	var lastPhase *app.BuildPhase
	var currentPhase *app.BuildPhase
	var failedPhase *app.BuildPhase
	var lastUpdate time.Time
	var status string

	ui.StartSpinner()
	buildStatus, err := a.GetBuildStatus(buildStatus.BuildNumber)
	if err != nil {
		return err
	}

	for {
		// catch up with any already completed phases
		if lastPhase == nil {
			logrus.Debug("setting current phase to Build")
			buildPhase := buildStatus.NamedPhases()[0]
			currentPhase = &buildPhase
		} else {
			failedPhase = buildStatus.FirstFailedPhase()
			if failedPhase != nil {
				return fmt.Errorf("%s failed at %s", failedPhase.Name, failedPhase.Phase.EndTime().Local().Format(timeFmt))
			}
			currentPhase = buildStatus.NextActivePhase(lastPhase)
		}

		// nothing is currently running
		if currentPhase == nil {
			logrus.WithFields(logrus.Fields{"phase": nil}).Debug("current phase")
			finalPhase, err := buildStatus.FinalPhase()
			if err != nil {
				logrus.Debug("waiting for the next phase to start")
				time.Sleep(5 * time.Second)

				continue
			}
			if finalPhase.Phase.State == "failed" {
				return fmt.Errorf("%s failed at %s", finalPhase.Name, finalPhase.Phase.EndTime().Local().Format(timeFmt))
			}
			if finalPhase.Name == "Deploy" {
				break
			}
			time.Sleep(5 * time.Second)

			continue
		}
		logrus.WithFields(logrus.Fields{"phase": currentPhase.Name}).Debug("current phase")
		lastUpdate = currentPhase.Phase.StartTime()
		// phase changed since last iteration
		if lastPhase == nil || lastPhase.Name != currentPhase.Name {
			caser := cases.Title(language.English)
			status = fmt.Sprintf("%s started", caser.String(currentPhase.Name))
			ui.Spinner.Stop()
			ui.Spinner.Suffix = ""
			if lastPhase != nil && lastPhase.Name == "Test" {
				// give test logs a chance to catch-up
				time.Sleep(1 * time.Second)
			}
			fmt.Printf("\n⚡️ %s\t%s\n", aurora.Yellow(status), aurora.Faint(currentPhase.Phase.StartTime().Local().Format(timeFmt)))
			ui.StartSpinner()
			lastPhase = currentPhase

			switch currentPhase.Name {
			case "Build":
				err = watchBuildPhase(a, buildStatus)
				if err != nil {
					return err
				}
				ui.Spinner.Suffix = ""
				ui.StartSpinner()
			case "Test":
				err = watchTestPhase(a, buildStatus)
				if err != nil {
					return err
				}
				ui.Spinner.Suffix = ""
				ui.StartSpinner()
			case "Release":
				err = watchReleasePhase(a, buildStatus)
				if err != nil {
					return err
				}
				ui.Spinner.Suffix = ""
				ui.StartSpinner()
			case "Postdeploy":
				err = watchPostdeployPhase(a, buildStatus)
				if err != nil {
					return err
				}
				ui.Spinner.Suffix = ""
				ui.StartSpinner()
			case "Deploy":
				err = watchDeployPhase(a, buildStatus)
				if err != nil {
					return err
				}
				ui.Spinner.Suffix = ""
				ui.StartSpinner()
			default:
				// sleep for a second so the spinner doesn't show "X running for now"
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			ui.Spinner.Suffix = fmt.Sprintf(" %s running for %s", currentPhase.Name, strings.Replace(humanize.Time(lastUpdate), " ago", "", 1))
		}
		// sleep for a bit if we're not ready to move onto the next phase
		if buildStatus.NextActivePhase(lastPhase) == lastPhase {
			time.Sleep(5 * time.Second)
			buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
			if err != nil {
				return err
			}
		}
	}
	ui.Spinner.Stop()
	printSuccess(fmt.Sprintf("build #%d deployed successfully", buildStatus.BuildNumber))
	return nil
}

func logMarker(name string) string {
	return fmt.Sprintf("#*#*#*#*#*# apppack-%s #*#*#*#*#*#", name)
}

func printLogLine(line string) {
	ui.Spinner.Stop()
	if strings.HasPrefix(line, "===> ") {
		fmt.Printf("%s %s\n", aurora.Blue("===>"), aurora.White(strings.SplitN(line, " ", 2)[1]))
	} else if strings.HasPrefix(line, "Unable to delete previous cache image: DELETE") {
		// https://github.com/aws/containers-roadmap/issues/1229
		logrus.WithFields(logrus.Fields{"line": line}).Debug("skipping inconsequential error")
	} else {
		fmt.Printf("%s\n", line)
	}
}

func S3Log(sess *session.Session, logURL string) error {
	contents, err := app.S3FromURL(sess, logURL)
	if err != nil {
		printWarning(fmt.Sprintf("unable to read log file: %s", logURL))
		return err
	}
	for _, l := range strings.Split(contents.String(), "\n") {
		printLogLine(l)
	}
	return nil
}

func StreamEvents(sess *session.Session, logURL string, marker *string, stopTailing <-chan bool) error {
	var lastSeenTime *int64
	var seenEventIDs map[string]bool
	var markerStart *string
	var markerStop *string
	var errorRe *regexp.Regexp
	markerFound := false
	cloudwatchlogsSvc := cloudwatchlogs.New(sess)
	parts := strings.Split(strings.TrimPrefix(logURL, "cloudwatch://"), "#")
	logGroup := parts[0]
	logStream := parts[1]
	logFields := logrus.Fields{
		"logGroup":  logGroup,
		"logStream": logStream,
	}
	if marker == nil {
		markerFound = true
	} else {
		markerStart = aws.String(logMarker(fmt.Sprintf("%s-start", *marker)))
		markerStop = aws.String(logMarker(fmt.Sprintf("%s-end", *marker)))
		errorRe = regexp.MustCompile(`^\[Container\] .* Command did not exit successfully .*`)
	}
	clearSeenEventIds := func() {
		seenEventIDs = make(map[string]bool)
	}

	addSeenEventIDs := func(id *string) {
		seenEventIDs[*id] = true
	}

	updateLastSeenTime := func(ts *int64) {
		if lastSeenTime == nil || *ts > *lastSeenTime {
			lastSeenTime = ts
			clearSeenEventIds()
		}
	}
	doneTailing := false
	handlePage := func(page *cloudwatchlogs.FilterLogEventsOutput, lastPage bool) bool {
		var message string
		for _, event := range page.Events {
			// messages may or may not have a newline. normalize them
			message = strings.TrimSuffix(*event.Message, "\n")
			updateLastSeenTime(event.Timestamp)
			if _, seen := seenEventIDs[*event.EventId]; !seen {
				if markerFound {
					if markerStop != nil && *markerStop == message {
						logrus.WithFields(logrus.Fields{
							"marker": *markerStop,
						}).Debug("found log marker")
						doneTailing = true
						return false
					}
					if errorRe != nil && errorRe.FindStringIndex(message) != nil {
						logrus.WithFields(logrus.Fields{
							"error": message,
						}).Debug("found error")
						doneTailing = true
						return false
					}
					printLogLine(message)
				} else if markerStart != nil {
					if *markerStart == strings.TrimSuffix(message, "\n") {
						logrus.WithFields(logrus.Fields{
							"marker": *markerStart,
						}).Debug("found log marker")
						markerFound = true
					}
				}
				addSeenEventIDs(event.EventId)
			}
		}
		return false
	}
	input := cloudwatchlogs.FilterLogEventsInput{LogGroupName: &logGroup, LogStreamNames: []*string{&logStream}}
	logrus.WithFields(logFields).Debug("starting log tail")
	stopSignalReceived := false
	countdown := 60

	for {
		select {
		case <-stopTailing:
			logrus.WithFields(logFields).Debug("log tail stop signal received")
			stopSignalReceived = true
		default:
			// log producer complete, but end marker not found
			if stopSignalReceived {
				if doneTailing {
					return nil
				}
				countdown--
			}
			if countdown < 0 {
				logrus.WithFields(logFields).Debug("end marker not found within countdown")
				return nil
			}
			if !doneTailing {
				err := cloudwatchlogsSvc.FilterLogEventsPages(
					&input,
					handlePage,
				)
				if err != nil {
					return err
				}
				if lastSeenTime != nil {
					input.SetStartTime(*lastSeenTime)
				}
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func watchBuildPhase(a *app.App, buildStatus *app.BuildStatus) error {
	ui.StartSpinner()
	buildStatus, err := a.GetBuildStatus(buildStatus.BuildNumber)
	if err != nil {
		return err
	}
	if strings.HasPrefix(buildStatus.Build.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Build.Logs)
	}
	codebuildSvc := codebuild.New(a.Session)
	buildLogTailing := false
	stopTailing := make(chan bool)

	for buildStatus.Build.State == Started {
		buildID := strings.Split(buildStatus.Build.Arns[0], "/")[1]
		builds, err := codebuildSvc.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
			Ids: []*string{&buildID},
		})
		if err != nil {
			return err
		}
		build := builds.Builds[0]
		if *build.CurrentPhase == "BUILD" {
			if strings.HasPrefix(buildStatus.Build.Logs, "s3://") {
				return S3Log(a.Session, buildStatus.Build.Logs)
			}
			if !buildLogTailing {
				buildLogTailing = true
				go StreamEvents(a.Session, buildStatus.Build.Logs, aws.String("build"), stopTailing)
			}
		} else if *build.CurrentPhase == "SUBMITTED" || *build.CurrentPhase == "QUEUED" || *build.CurrentPhase == "PROVISIONING" || *build.CurrentPhase == "DOWNLOAD_SOURCE" || *build.CurrentPhase == "INSTALL" || *build.CurrentPhase == "PRE_BUILD" {
			ui.StartSpinner()
			caser := cases.Title(language.English)
			ui.Spinner.Suffix = fmt.Sprintf(" CodeBuild phase: %s", caser.String(strings.ToLower(strings.ReplaceAll(*build.CurrentPhase, "_", " "))))
		} else {
			logrus.WithFields(logrus.Fields{"phase": *build.CurrentPhase}).Debug("watch build stopped")
			if buildLogTailing {
				stopTailing <- true
			}
			return nil
		}
		time.Sleep(5 * time.Second)
		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return err
		}
	}
	return nil
}

func watchTestPhase(a *app.App, buildStatus *app.BuildStatus) error {
	ui.StartSpinner()
	stopTailing := make(chan bool)
	buildStatus, err := a.GetBuildStatus(buildStatus.BuildNumber)
	if err != nil {
		return err
	}
	if strings.HasPrefix(buildStatus.Test.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Test.Logs)
	}
	go StreamEvents(a.Session, buildStatus.Build.Logs, aws.String("test"), stopTailing)

	for buildStatus.Test.State == Started {
		time.Sleep(5 * time.Second)
		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return err
		}
	}
	stopTailing <- true
	return nil
}

func watchReleasePhase(a *app.App, buildStatus *app.BuildStatus) error {
	ui.StartSpinner()
	stopTailing := make(chan bool)
	buildStatus, err := a.GetBuildStatus(buildStatus.BuildNumber)
	if err != nil {
		return err
	}
	ecsSvc := ecs.New(a.Session)
	if err = a.LoadSettings(); err != nil {
		return err
	}
	releaseLogTailing := false
	if strings.HasPrefix(buildStatus.Release.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Release.Logs)
	}
	for buildStatus.Release.State == Started {
		if len(buildStatus.Release.Arns) > 0 {
			out, err := ecsSvc.DescribeTasks(&ecs.DescribeTasksInput{
				Cluster: &a.Settings.Cluster.ARN,
				Tasks:   []*string{&buildStatus.Release.Arns[0]},
			})
			if err != nil {
				return err
			}
			status := *out.Tasks[0].LastStatus
			if status == "RUNNING" && !releaseLogTailing {
				releaseLogTailing = true
				go StreamEvents(a.Session, buildStatus.Release.Logs, nil, stopTailing)
			}
			if status == "DEACTIVATING" || status == "STOPPING" || status == "DEPROVISIONING" || status == "STOPPED" {
				stopTailing <- true
				ui.StartSpinner()
			}
			caser := cases.Title(language.English)
			ui.Spinner.Suffix = fmt.Sprintf(" ECS task status: %s", caser.String(strings.ToLower(status)))
		}
		time.Sleep(5 * time.Second)
		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return err
		}
	}
	logrus.Debug("loop end")
	logrus.WithFields(logrus.Fields{"phase": "release"}).Debug("phase completed")
	return nil
}

// TODO DRY with watchReleasePhase. Combine to watchEcsTaskPhase
func watchPostdeployPhase(a *app.App, buildStatus *app.BuildStatus) error {
	ui.StartSpinner()
	stopTailing := make(chan bool)
	buildStatus, err := a.GetBuildStatus(buildStatus.BuildNumber)
	if err != nil {
		return err
	}
	ecsSvc := ecs.New(a.Session)
	if err = a.LoadSettings(); err != nil {
		return err
	}
	postdeployLogTailing := false
	if strings.HasPrefix(buildStatus.Postdeploy.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Postdeploy.Logs)
	}
	for buildStatus.Postdeploy.State == Started {
		if len(buildStatus.Postdeploy.Arns) > 0 {
			out, err := ecsSvc.DescribeTasks(&ecs.DescribeTasksInput{
				Cluster: &a.Settings.Cluster.ARN,
				Tasks:   []*string{&buildStatus.Postdeploy.Arns[0]},
			})
			if err != nil {
				return err
			}
			status := *out.Tasks[0].LastStatus
			if status == "RUNNING" && !postdeployLogTailing {
				postdeployLogTailing = true
				go StreamEvents(a.Session, buildStatus.Postdeploy.Logs, nil, stopTailing)
			}
			if status == "DEACTIVATING" || status == "STOPPING" || status == "DEPROVISIONING" || status == "STOPPED" {
				stopTailing <- true
				ui.StartSpinner()
			}
			caser := cases.Title(language.English)
			ui.Spinner.Suffix = fmt.Sprintf(" ECS task status: %s", caser.String(strings.ToLower(status)))
		}
		time.Sleep(5 * time.Second)
		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return err
		}
	}
	return nil
}

func streamEcsServiceEvents(a *app.App, buildStatus *app.BuildStatus) error {
	buildStatus, err := a.GetBuildStatus(buildStatus.BuildNumber)
	if err != nil {
		return err
	}
	serviceARNs := buildStatus.Deploy.Arns
	ecsSvc := ecs.New(a.Session)
	if err = a.LoadSettings(); err != nil {
		return err
	}
	seenEventIDs := map[string]bool{}
	var serviceStatus *ecs.DescribeServicesOutput

	for buildStatus.Deploy.State == Started {
		logrus.WithFields(logrus.Fields{
			"services": serviceARNs,
		}).Debug("polling service status")
		serviceStatus, err = ecsSvc.DescribeServices(&ecs.DescribeServicesInput{
			Cluster:  &a.Settings.Cluster.ARN,
			Services: aws.StringSlice(serviceARNs),
		})
		if err != nil {
			return err
		}
		for _, service := range serviceStatus.Services {
			eventCount := len(service.Events)
			for i := range service.Events {
				// iterate slice in reverse
				event := service.Events[eventCount-1-i]
				if _, seen := seenEventIDs[*event.Id]; seen {
					continue
				} else if event.CreatedAt.Unix() < buildStatus.Deploy.Start {
					logrus.WithFields(logrus.Fields{
						"apppack": buildStatus.Deploy.Start,
						"ecs":     event.CreatedAt.Unix(),
					}).Debug("skipping event before deploy")
					seenEventIDs[*event.Id] = true

					continue
				}
				seenEventIDs[*event.Id] = true
				ui.Spinner.Stop()
				fmt.Printf("%s\n", *event.Message)
			}
		}
		ui.StartSpinner()
		time.Sleep(5 * time.Second)
		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return err
		}
	}
	return nil
}

func watchDeployPhase(a *app.App, buildStatus *app.BuildStatus) error {
	ui.StartSpinner()
	return streamEcsServiceEvents(a, buildStatus)
}

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:                   "build",
	Short:                 "work with AppPack builds",
	Long:                  `Use subcommands to view, list, and trigger code builds.`,
	DisableFlagsInUseLine: true,
}

// buildStartCmd represents the start command
var buildStartCmd = &cobra.Command{
	Use:                   "start",
	Short:                 "start a new build from the latest commit on the branch defined in AppPack",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		var duration int
		if watchBuildFlag {
			duration = MaxSessionDurationSeconds
		} else {
			duration = SessionDurationSeconds
		}
		a, err := app.Init(AppName, UseAWSCredentials, duration)
		checkErr(err)
		if a.Pipeline && a.ReviewApp == nil {
			err := fmt.Errorf("%q is a pipeline. You can build ReviewApps within a pipeline", a.Name)
			checkErr(err)
		}
		_, err = a.ReviewAppExists()
		checkErr(err)
		build, err := a.StartBuild(false)
		checkErr(err)
		ui.Spinner.Stop()
		printSuccess("build started")
		ui.StartSpinner()
		buildStatus, err := pollBuildStatus(a, int(*build.BuildNumber), 10)
		checkErr(err)
		ui.Spinner.Stop()
		printBuild(buildStatus)
		if watchBuildFlag {
			checkErr(watchBuild(a, buildStatus))
		}
	},
}

// buildWaitCmd represents the start command
var buildWaitCmd = &cobra.Command{
	Use:                   "wait",
	Short:                 "wait is deprecated -- use `watch` instead",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		checkErr(fmt.Errorf("the `build wait` command is deprecated -- use `build watch` instead"))
	},
}

// buildWaitCmd represents the start command
var buildWatchCmd = &cobra.Command{
	Use:                   "watch [<build-number>]",
	Short:                 "watch the progress of the most recent build",
	Args:                  cobra.MaximumNArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		var build *app.BuildStatus
		var buildNumber int
		if len(args) > 0 {
			buildNumber, err = strconv.Atoi(args[0])
			checkErr(err)
			build, err = a.GetBuildStatus(buildNumber)
		} else {
			build, err = a.GetBuildStatus(-1)
		}
		checkErr(err)
		ui.Spinner.Stop()
		printBuild(build)
		printCommitLog(a.Session, build)
		checkErr(watchBuild(a, build))
	},
}

// buildListCmd represents the list command
var buildListCmd = &cobra.Command{
	Use:                   "list",
	Short:                 "list recent builds",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		builds, err := a.RecentBuilds(15)
		checkErr(err)
		ui.Spinner.Stop()
		for i := range builds {
			printBuild(&builds[i])
			printCommitLog(a.Session, &builds[i])
		}
	},
}

var watchBuildFlag bool

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	buildCmd.MarkPersistentFlagRequired("app-name")
	buildCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")

	buildCmd.AddCommand(buildStartCmd)
	buildStartCmd.Flags().BoolVarP(&watchBuildFlag, "watch", "w", false, "watch build process")
	buildStartCmd.Flags().BoolVar(&watchBuildFlag, "wait", false, "watch build process")
	buildStartCmd.Flags().MarkDeprecated("wait", "please use --watch instead")
	buildCmd.AddCommand(buildListCmd)

	buildCmd.AddCommand(buildWaitCmd)
	buildCmd.AddCommand(buildWatchCmd)
}
