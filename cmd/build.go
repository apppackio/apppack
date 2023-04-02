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
	"os"
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
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	Started      = "started"
	PollInterval = 5 * time.Second
)

var (
	BuildListCount            int
	watchBuildFlag            bool
	ErrBuildFailed            = fmt.Errorf("build failed")
	ErrBuildWaitDeprecated    = fmt.Errorf("build wait is deprecated, use `build watch` instead")
	ErrTaskNotFound           = fmt.Errorf("task not found")
	ErrCodebuildBuildNotFound = fmt.Errorf("codebuild build not found")
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

func printCommitLog(sess *session.Session, buildStatus *app.BuildStatus) {
	commitLog, err := buildStatus.GetCommitLog(sess)
	if err == nil {
		fmt.Println(indent(*commitLog, ui.Indent))
	} else {
		fmt.Print(ui.Indent)
		printWarning(fmt.Sprintf("unable to read commit data for build #%d", buildStatus.BuildNumber))
		fmt.Println()
	}
}

func pollBuildStatus(a *app.App, buildNumber, retries int) (*app.BuildStatus, error) {
	// retry a few times for the initial status to show up
	// once it does, stop retrying
	var (
		buildStatus *app.BuildStatus
		err         error
	)

	for range time.Tick(3 * time.Second) {
		buildStatus, err = a.GetBuildStatus(buildNumber)

		if retries == 0 {
			return buildStatus, err
		}

		if err != nil {
			retries--

			continue
		}

		break
	}

	return buildStatus, nil
}

// handlePhase handles watching a specific phase of the build
func handlePhase(a *app.App, buildStatus *app.BuildStatus, phaseName app.BuildPhaseLabel) error {
	var err error

	handlers := map[app.BuildPhaseLabel]func(*app.App, *app.BuildStatus, app.BuildPhaseLabel) error{
		app.BuildPhaseName:      watchBuildPhase,
		app.TestPhaseName:       watchTestPhase,
		app.FinalizePhaseName:   func(*app.App, *app.BuildStatus, app.BuildPhaseLabel) error { return nil },
		app.ReleasePhaseName:    watchEcsTaskPhase,
		app.PostdeployPhaseName: watchEcsTaskPhase,
		app.DeployPhaseName:     watchDeployPhase,
	}

	if handler, ok := handlers[phaseName]; ok {
		logrus.WithField("phase", phaseName).Debug("starting phase handler")
		if err = handler(a, buildStatus, phaseName); err != nil {
			return err
		}

		ui.Spinner.Suffix = ""

		ui.StartSpinner()
	} else {
		// sleep so the spinner doesn't show "X running for now"
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func waitForNextPhase(a *app.App, buildStatus *app.BuildStatus, lastPhase *app.BuildPhase) (*app.BuildStatus, *app.BuildPhase, error) {
	var err error

	currentPhase := buildStatus.NextActivePhase(lastPhase)
	if currentPhase.Name != lastPhase.Name {
		return buildStatus, currentPhase, nil
	}

	// wait for the next phase
	for range time.Tick(PollInterval) {
		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return nil, nil, err
		}

		currentPhase = buildStatus.NextActivePhase(lastPhase)
		if currentPhase != lastPhase {
			break
		}
	}

	return buildStatus, currentPhase, nil
}

func watchBuild(a *app.App, buildStatus *app.BuildStatus) error {
	var (
		lastPhase   *app.BuildPhase
		failedPhase *app.BuildPhase
		status      string
	)

	ui.StartSpinner()

	buildStatus, err := a.GetBuildStatus(buildStatus.BuildNumber)
	if err != nil {
		return err
	}
	// always start at the build phase
	currentPhase := buildStatus.NamedPhases()[0]

	for {
		// bail out if there is a failure
		failedPhase = buildStatus.FirstFailedPhase()
		if failedPhase != nil {
			return fmt.Errorf(
				"%s failed at %s %w",
				failedPhase.Name,
				failedPhase.Phase.EndTime().Local().Format(ui.TimeFmt),
				ErrBuildFailed,
			)
		}

		// nothing is currently running
		if currentPhase != nil {

			logrus.WithFields(logrus.Fields{"phase": currentPhase.Name}).Debug("current phase")

			// phase changed since last iteration
			if lastPhase == nil || lastPhase.Name != currentPhase.Name {
				status = fmt.Sprintf("%s started", currentPhase.Name)

				ui.Spinner.Stop()
				ui.Spinner.Suffix = ""

				if lastPhase != nil && lastPhase.Name == app.TestPhaseName && buildStatus.Status == app.PhaseInProgress {
					// give test logs a chance to catch-up
					time.Sleep(1 * time.Second)
				}

				fmt.Printf(
					"\n⚡️ %s\t%s\n",
					aurora.Yellow(status),
					aurora.Faint(currentPhase.Phase.StartTime().Local().Format(ui.TimeFmt)),
				)
				ui.StartSpinner()

				if err = handlePhase(a, buildStatus, currentPhase.Name); err != nil {
					return err
				}

				if currentPhase.Name == app.DeployPhaseName {
					break
				}
			}
		}

		lastPhase = currentPhase
		buildStatus, currentPhase, err = waitForNextPhase(a, buildStatus, lastPhase)
		if err != nil {
			return err
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

	switch {
	case strings.HasPrefix(line, "===> "):
		fmt.Printf("%s %s\n", aurora.Blue("===>"), aurora.White(strings.SplitN(line, " ", 2)[1]))
	case strings.HasPrefix(line, "Unable to delete previous cache image: DELETE"):
		// https://github.com/aws/containers-roadmap/issues/1229
		logrus.WithFields(logrus.Fields{"line": line}).Debug("skipping inconsequential error")
	default:
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

// StreamEvents fetches events from a CloudWatch log group and streams all events between the markers to stdout
// until the stopTailing channel is closed.
func StreamEvents(sess *session.Session, logURL string, marker *string, stopTailing <-chan bool) error {
	var (
		lastSeenTime *int64
		seenEventIDs map[string]bool
		markerStart  *string
		markerStop   *string
		errorRe      *regexp.Regexp
	)

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
			if _, seen := seenEventIDs[*event.EventId]; seen {
				continue
			}
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

// hasHiddenBuildFailure returns true if the build failed outside of BUILD or POST_BUILD phases
func hasHiddenBuildFailure(build *codebuild.Build) *codebuild.BuildPhase {
	for _, phase := range build.Phases {
		if phase == nil || phase.PhaseType == nil || phase.PhaseStatus == nil {
			continue
		}

		hidden := !(*phase.PhaseType == "BUILD" || *phase.PhaseType == "POST_BUILD")
		failed := !(*phase.PhaseStatus == "IN_PROGRESS" || *phase.PhaseStatus == "SUCCEEDED")

		if hidden && failed {
			return phase
		}
	}
	return nil
}

// dumpCodebuildLogForPhase prints the logs for a given phase to stdout.
func dumpCodebuildLogForPhase(sess *session.Session, build *codebuild.Build, phase *codebuild.BuildPhase) error {
	logsSvc := cloudwatchlogs.New(sess)
	return logsSvc.GetLogEventsPages(&cloudwatchlogs.GetLogEventsInput{
		StartTime:     aws.Int64(phase.StartTime.UnixMilli()),
		EndTime:       aws.Int64(phase.EndTime.UnixMilli() + 5000), // add 5 seconds to ensure we get all the logs
		LogGroupName:  build.Logs.GroupName,
		LogStreamName: build.Logs.StreamName,
	}, func(page *cloudwatchlogs.GetLogEventsOutput, _ bool) bool {
		for _, event := range page.Events {
			printLogLine(*event.Message)
		}
		return true
	})
}

// loadCodebuildBuild loads the Codebuild build from the AWS API.
func loadCodebuildBuild(codebuildSvc codebuild.CodeBuild, buildARN string) (*codebuild.Build, error) {
	buildID := strings.Split(buildARN, "/")[1]
	builds, err := codebuildSvc.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
		Ids: []*string{&buildID},
	})
	if err != nil {
		return nil, err
	}

	if len(builds.Builds) == 0 {
		return nil, ErrCodebuildBuildNotFound
	}

	return builds.Builds[0], nil
}

// watchBuildPhase watches the Codebuild process and prints the logs to the console.
func watchBuildPhase(a *app.App, buildStatus *app.BuildStatus, _ app.BuildPhaseLabel) error {
	ui.StartSpinner()

	// bail early if the logs are already on S3
	if strings.HasPrefix(buildStatus.Build.Logs, "s3://") {
		return S3Log(a.Session, buildStatus.Build.Logs)
	}

	codebuildSvc := codebuild.New(a.Session)
	buildLogTailing := false
	stopTailing := make(chan bool)

	build, err := loadCodebuildBuild(*codebuildSvc, buildStatus.Build.Arns[0])
	if err != nil {
		return err
	}

	failedAt := hasHiddenBuildFailure(build)
	if failedAt != nil {
		return dumpCodebuildLogForPhase(a.Session, build, failedAt)
	}

	for range time.Tick(PollInterval) {
		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return err
		}

		if buildStatus.Build.State != Started {
			return nil
		}

		build, err := loadCodebuildBuild(*codebuildSvc, buildStatus.Build.Arns[0])
		if err != nil {
			return err
		}

		failedAt := hasHiddenBuildFailure(build)
		if failedAt != nil {
			return dumpCodebuildLogForPhase(a.Session, build, failedAt)
		}

		switch {
		case *build.CurrentPhase == "BUILD":
			if strings.HasPrefix(buildStatus.Build.Logs, "s3://") {
				return S3Log(a.Session, buildStatus.Build.Logs)
			}
			if !buildLogTailing {
				buildLogTailing = true
				go StreamEvents(a.Session, buildStatus.Build.Logs, aws.String("build"), stopTailing)
			}
		case *build.CurrentPhase == "SUBMITTED" || *build.CurrentPhase == "QUEUED" || *build.CurrentPhase == "PROVISIONING" || *build.CurrentPhase == "DOWNLOAD_SOURCE" || *build.CurrentPhase == "INSTALL" || *build.CurrentPhase == "PRE_BUILD":
			ui.StartSpinner()

			caser := cases.Title(language.English)
			ui.Spinner.Suffix = fmt.Sprintf(" CodeBuild phase: %s", caser.String(strings.ToLower(strings.ReplaceAll(*build.CurrentPhase, "_", " "))))
		default:
			logrus.WithFields(logrus.Fields{"phase": *build.CurrentPhase}).Debug("watch build stopped")
			if buildLogTailing {
				stopTailing <- true
			}

			return nil
		}
	}

	return nil
}

func watchTestPhase(a *app.App, buildStatus *app.BuildStatus, _ app.BuildPhaseLabel) error {
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

	for range time.Tick(PollInterval) {
		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return err
		}

		if buildStatus.Test.State != Started {
			break
		}
	}

	stopTailing <- true

	return nil
}

// getTaskStatus returns the last status of a task from ECS
func getTaskStatus(ecsSvc *ecs.ECS, clusterArn string, taskArn string) (string, error) {
	task, err := ecsSvc.DescribeTasks(&ecs.DescribeTasksInput{
		Tasks:   []*string{&taskArn},
		Cluster: &clusterArn,
	})
	if err != nil {
		return "", err
	}
	if len(task.Tasks) == 0 {
		return "", ErrTaskNotFound
	}
	return *task.Tasks[0].LastStatus, nil
}

// fetchBuildPhase fetches the build status from DDB and returns the specified phase by name
func fetchBuildPhase(a *app.App, buildNumber int, phaseName app.BuildPhaseLabel) (*app.BuildPhaseDetail, error) {
	buildStatus, err := a.GetBuildStatus(buildNumber)
	if err != nil {
		return nil, err
	}

	return buildStatus.PhaseByName(phaseName)
}

func watchEcsTaskPhase(a *app.App, buildStatus *app.BuildStatus, phaseName app.BuildPhaseLabel) error {
	ui.StartSpinner()

	stopTailing := make(chan bool)
	isTailing := false

	phase, err := buildStatus.PhaseByName(phaseName)
	if err != nil {
		return err
	}

	// if the logs are in s3, just return those
	if strings.HasPrefix(phase.Logs, "s3://") {
		return S3Log(a.Session, phase.Logs)
	}

	ecsSvc := ecs.New(a.Session)

	if err = a.LoadSettings(); err != nil {
		return err
	}

	for range time.Tick(PollInterval) {
		phase, err = fetchBuildPhase(a, buildStatus.BuildNumber, phaseName)
		if err != nil {
			return err
		}
		// only watch while the phase is in progress
		if phase.State != Started {
			return nil
		}
		// if a task hasn't been created yet, just keep waiting
		if len(phase.Arns) == 0 {
			continue
		}

		status, err := getTaskStatus(ecsSvc, a.Settings.Cluster.ARN, phase.Arns[0])
		if err != nil {
			return err
		}

		if status == "RUNNING" && !isTailing {
			isTailing = true
			go StreamEvents(a.Session, phase.Logs, nil, stopTailing)
		}

		if status == "DEACTIVATING" || status == "STOPPING" || status == "DEPROVISIONING" || status == "STOPPED" {
			stopTailing <- true

			ui.StartSpinner()
		}

		caser := cases.Title(language.English)
		ui.Spinner.Suffix = fmt.Sprintf(" ECS task status: %s", caser.String(strings.ToLower(status)))
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
	seenEventIDs := map[string]bool{}

	if err = a.LoadSettings(); err != nil {
		return err
	}

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
		time.Sleep(PollInterval)

		buildStatus, err = a.GetBuildStatus(buildStatus.BuildNumber)
		if err != nil {
			return err
		}
	}
	return nil
}

func watchDeployPhase(a *app.App, buildStatus *app.BuildStatus, _ app.BuildPhaseLabel) error {
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
		build, err := a.StartBuild(false)
		checkErr(err)
		ui.Spinner.Stop()
		printSuccess("build started")
		ui.StartSpinner()
		buildStatus, err := pollBuildStatus(a, int(*build.BuildNumber), 10)
		checkErr(err)
		ui.Spinner.Stop()
		buildStatus.ToConsole(os.Stdout)
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
		checkErr(ErrBuildWaitDeprecated)
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
		build.ToConsole(os.Stdout)
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
		builds, err := a.RecentBuilds(BuildListCount)
		checkErr(err)
		ui.Spinner.Stop()

		if AsJSON {
			buf, err := builds.ToJSON()
			checkErr(err)
			fmt.Println(buf.String())
			return
		}

		for i := range builds {
			builds[i].ToConsole(os.Stdout)
			printCommitLog(a.Session, builds[i])
		}
	},
}

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
	buildListCmd.Flags().IntVarP(&BuildListCount, "num", "n", 15, "limit number of builds to list")
	buildListCmd.Flags().BoolVarP(&AsJSON, "json", "j", false, "output as JSON")

	buildCmd.AddCommand(buildWaitCmd)
	buildCmd.AddCommand(buildWatchCmd)
}
