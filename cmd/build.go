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
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
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

func retryBuildStatus(a *app.App, build *codebuild.Build, retries int) (*app.BuildStatus, error) {
	// retry a few times for the initial status to show up
	// once it does, stop retrying
	buildStatus, err := a.BuildStatus(build)
	if err != nil {
		if retries > 0 {
			time.Sleep(3 * time.Second)
			return retryBuildStatus(a, build, retries-1)
		} else {
			return nil, err
		}
	}
	return buildStatus, nil
}

func watchBuild(a *app.App, build *codebuild.Build) error {
	var lastPhase *app.BuildPhase
	var currentPhase *app.BuildPhase
	var lastUpdate time.Time
	var status string
	startSpinner()
	buildStatus, err := retryBuildStatus(a, build, 10)
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
			if lastPhase.Phase.State == "failed" {
				return fmt.Errorf("%s failed at %s", lastPhase.Name, time.Unix(lastPhase.Phase.End, 0).Local().Format(timeFmt))
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
				return fmt.Errorf("%s failed at %s", finalPhase.Name, time.Unix(finalPhase.Phase.End, 0).Local().Format(timeFmt))
			}
			if finalPhase.Name == "Deploy" {
				break
			} else {
				time.Sleep(5 * time.Second)
				continue
			}
		} else {
			logrus.WithFields(logrus.Fields{"phase": currentPhase.Name}).Debug("current phase")
		}
		lastUpdate = time.Unix(currentPhase.Phase.Start, 0)
		// phase changed since last iteration
		if lastPhase == nil || lastPhase.Name != currentPhase.Name {
			status = fmt.Sprintf("%s started", strings.Title(currentPhase.Name))
			Spinner.Stop()
			Spinner.Suffix = ""
			fmt.Printf("\n⚡️ %s\t%s\n", aurora.Yellow(status), aurora.Faint(time.Unix(currentPhase.Phase.Start, 0).Local().Format(timeFmt)))
			startSpinner()
			lastPhase = currentPhase
			switch currentPhase.Name {
			case "Build":
				err = watchBuildPhase(a, build)
				if err != nil {
					return err
				}
				Spinner.Suffix = ""
				startSpinner()
			case "Test":
				err = watchTestPhase(a, build)
				if err != nil {
					return err
				}
				Spinner.Suffix = ""
				startSpinner()
			case "Release":
				err = watchReleasePhase(a, build)
				if err != nil {
					return err
				}
				Spinner.Suffix = ""
				startSpinner()
			case "Postdeploy":
				err = watchPostdeployPhase(a, build)
				if err != nil {
					return err
				}
				Spinner.Suffix = ""
				startSpinner()
			case "Deploy":
				err = watchDeployPhase(a, build)
				if err != nil {
					return err
				}
				Spinner.Suffix = ""
				startSpinner()
			default:
				// sleep for a second so the spinner doesn't show "X running for now"
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			Spinner.Suffix = fmt.Sprintf(" %s running for %s", currentPhase.Name, strings.Replace(humanize.Time(lastUpdate), " ago", "", 1))
		}
		// sleep for a bit if we're not ready to move onto the next phase
		if buildStatus.NextActivePhase(lastPhase) == lastPhase {
			time.Sleep(5 * time.Second)
			buildStatus, err = a.BuildStatus(build)
			if err != nil {
				return err
			}
		}
	}
	Spinner.Stop()
	printSuccess(fmt.Sprintf("build #%d deployed successfully", *build.BuildNumber))
	return nil
}

func logMarker(name string) string {
	return fmt.Sprintf("#*#*#*#*#*# apppack-%s #*#*#*#*#*#", name)
}

var stopTailing = make(chan bool, 1)

func printLogLine(line string) {
	Spinner.Stop()
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
	s3Svc := s3.New(sess)
	parts := strings.Split(strings.TrimPrefix(logURL, "s3://"), "/")
	bucket := parts[0]
	object := strings.Join(parts[1:], "/")
	out, err := s3Svc.GetObject(&s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &object,
	})
	if err != nil {
		return err
	}
	buf := new(strings.Builder)
	_, err = io.Copy(buf, out.Body)
	if err != nil {
		return err
	}
	for _, l := range strings.Split(buf.String(), "\n") {
		printLogLine(l)
	}
	return nil
}

func StreamEvents(sess *session.Session, logURL string, marker *string) error {
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
		errorRe = regexp.MustCompile(`^[Container] .* Command did not exit successfully .*`)
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
				} else {
					if markerStart != nil {
						if *markerStart == strings.TrimSuffix(message, "\n") {
							logrus.WithFields(logrus.Fields{
								"marker": *markerStart,
							}).Debug("found log marker")
							markerFound = true
						}
					}
				}
				addSeenEventIDs(event.EventId)
			}
		}
		return !lastPage
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

func watchBuildPhase(a *app.App, build *codebuild.Build) error {
	startSpinner()
	buildStatus, err := a.BuildStatus(build)
	if err != nil {
		return err
	}
	if strings.HasPrefix(buildStatus.Build.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Build.Logs)
	}
	codebuildSvc := codebuild.New(a.Session)
	buildLogTailing := false
	for buildStatus.Build.State == "started" {
		builds, err := codebuildSvc.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
			Ids: []*string{build.Id},
		})
		if err != nil {
			return err
		}
		build = builds.Builds[0]
		if *build.CurrentPhase == "BUILD" {
			if !buildLogTailing {
				buildLogTailing = true
				go StreamEvents(a.Session, buildStatus.Build.Logs, aws.String("build"))
			}
		} else if *build.CurrentPhase == "SUBMITTED" || *build.CurrentPhase == "QUEUED" || *build.CurrentPhase == "PROVISIONING" || *build.CurrentPhase == "DOWNLOAD_SOURCE" || *build.CurrentPhase == "INSTALL" || *build.CurrentPhase == "PRE_BUILD" {
			startSpinner()
			Spinner.Suffix = fmt.Sprintf(" CodeBuild phase: %s", strings.Title(strings.ToLower(strings.Replace(*build.CurrentPhase, "_", " ", -1))))
		} else {
			logrus.WithFields(logrus.Fields{"phase": *build.CurrentPhase}).Debug("watch build stopped")
			if buildLogTailing {
				stopTailing <- true
			}
			return nil

		}
		time.Sleep(5 * time.Second)
		buildStatus, err = a.BuildStatus(build)
		if err != nil {
			return err
		}
	}
	return nil
}

func watchTestPhase(a *app.App, build *codebuild.Build) error {
	startSpinner()
	buildStatus, err := a.BuildStatus(build)
	if err != nil {
		return err
	}
	if strings.HasPrefix(buildStatus.Test.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Test.Logs)
	}
	go StreamEvents(a.Session, buildStatus.Build.Logs, aws.String("test"))
	for buildStatus.Test.State == "started" {
		time.Sleep(5 * time.Second)
		buildStatus, err = a.BuildStatus(build)
		if err != nil {
			return err
		}
	}
	stopTailing <- true
	return nil
}

func watchReleasePhase(a *app.App, build *codebuild.Build) error {
	startSpinner()
	buildStatus, err := a.BuildStatus(build)
	if err != nil {
		return err
	}
	ecsSvc := ecs.New(a.Session)
	a.LoadSettings()
	releaseLogTailing := false
	if strings.HasPrefix(buildStatus.Release.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Release.Logs)
	}
	for buildStatus.Release.State == "started" {
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
				go StreamEvents(a.Session, buildStatus.Release.Logs, nil)
			}
			if status == "DEACTIVATING" || status == "STOPPING" || status == "DEPROVISIONING" || status == "STOPPED" {
				stopTailing <- true
				startSpinner()
			}
			Spinner.Suffix = fmt.Sprintf(" ECS task status: %s", strings.Title(strings.ToLower(status)))
		}
		time.Sleep(5 * time.Second)
		buildStatus, err = a.BuildStatus(build)
		if err != nil {
			return err
		}
	}
	return nil
}

// TODO DRY with watchReleasePhase. Combine to watchEcsTaskPhase
func watchPostdeployPhase(a *app.App, build *codebuild.Build) error {
	startSpinner()
	buildStatus, err := a.BuildStatus(build)
	if err != nil {
		return err
	}
	ecsSvc := ecs.New(a.Session)
	a.LoadSettings()
	postdeployLogTailing := false
	if strings.HasPrefix(buildStatus.Postdeploy.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Postdeploy.Logs)
	}
	for buildStatus.Postdeploy.State == "started" {
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
				go StreamEvents(a.Session, buildStatus.Postdeploy.Logs, nil)
			}
			if status == "DEACTIVATING" || status == "STOPPING" || status == "DEPROVISIONING" || status == "STOPPED" {
				stopTailing <- true
				startSpinner()
			}
			Spinner.Suffix = fmt.Sprintf(" ECS task status: %s", strings.Title(strings.ToLower(status)))
		}
		time.Sleep(5 * time.Second)
		buildStatus, err = a.BuildStatus(build)
		if err != nil {
			return err
		}
	}
	return nil
}

func streamEcsServiceEvents(a *app.App, build *codebuild.Build, serviceArns []string) error {
	buildStatus, err := a.BuildStatus(build)
	if err != nil {
		return err
	}
	ecsSvc := ecs.New(a.Session)
	a.LoadSettings()
	seenEventIDs := map[string]bool{}
	var serviceStatus *ecs.DescribeServicesOutput
	for buildStatus.Deploy.State == "started" {
		logrus.WithFields(logrus.Fields{
			"services": serviceArns,
		}).Debug("polling service status")
		serviceStatus, err = ecsSvc.DescribeServices(&ecs.DescribeServicesInput{
			Cluster:  &a.Settings.Cluster.ARN,
			Services: aws.StringSlice(serviceArns),
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
				Spinner.Stop()
				fmt.Printf("%s\n", *event.Message)
			}
		}
		startSpinner()
		time.Sleep(5 * time.Second)
		buildStatus, err = a.BuildStatus(build)
		if err != nil {
			return err
		}
	}
	return nil
}

func watchDeployPhase(a *app.App, build *codebuild.Build) error {
	startSpinner()
	buildStatus, err := a.BuildStatus(build)
	if err != nil {
		return err
	}
	return streamEcsServiceEvents(a, build, buildStatus.Deploy.Arns)
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
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		build, err := a.StartBuild(false)
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
	Use:                   "wait",
	Short:                 "wait for the most recent build to be deployed",
	DisableFlagsInUseLine: true,
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
	Use:                   "list",
	Short:                 "list recent builds",
	DisableFlagsInUseLine: true,
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
	buildCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	buildCmd.MarkPersistentFlagRequired("app-name")

	buildCmd.AddCommand(buildStartCmd)
	buildStartCmd.Flags().BoolVarP(&watchBuildFlag, "wait", "w", false, "wait for build to complete")
	buildCmd.AddCommand(buildListCmd)

	buildCmd.AddCommand(buildWaitCmd)
}
