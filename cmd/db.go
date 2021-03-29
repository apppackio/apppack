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
	"strings"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

func downloadFile(sess *session.Session, objInput *s3.GetObjectInput, outputFile string) error {
	Spinner.Suffix = fmt.Sprintf(" downloading %s", outputFile)
	downloader := s3manager.NewDownloader(sess)
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	_, err = downloader.Download(file, objInput)
	if err != nil {
		return err
	}
	return nil
}

func uploadFile(sess *session.Session, uploadInput *s3manager.UploadInput) error {
	uploader := s3manager.NewUploader(sess)
	_, err := uploader.Upload(uploadInput)
	if err != nil {
		return err
	}
	return nil
}

// dbCmd represents the db command
var dbCmd = &cobra.Command{
	Use:                   "db",
	Short:                 "tools to manage your database on AppPack",
	DisableFlagsInUseLine: true,
}

// dbShellCmd represents the db shell command
var dbShellCmd = &cobra.Command{
	Use:                   "shell",
	Short:                 "open an interactive shell prompt to the app database",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		family, exec, err := a.DBShellTaskInfo()
		checkErr(err)
		StartInteractiveShell(a, family, aws.String(fmt.Sprintf("entrypoint.sh %s", *exec)))
	},
}

// dbDumpCmd represents the db load command
var dbDumpCmd = &cobra.Command{
	Use:                   "dump",
	Short:                 "dump the database to a local file",
	Long:                  "Dump the database to `<app-name>.dump` in the current directory",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		app, err := app.Init(AppName)
		checkErr(err)
		err = app.LoadSettings()
		checkErr(err)
		task, getObjectInput, err := app.DBDump()
		checkErr(err)
		Spinner.Suffix = fmt.Sprintf(" dumping database %s", aurora.Faint(*task.TaskArn))
		exitCode, err := app.WaitForTaskStopped(task)
		checkErr(err)
		if *exitCode != 0 {
			taskLogs(app.Session, task)
			printError("database dump failed")
			return
		}
		localFile := fmt.Sprintf("%s.dump", app.Name)
		err = downloadFile(app.Session, getObjectInput, localFile)
		checkErr(err)
		Spinner.Stop()
		printSuccess(fmt.Sprintf("Dumped database to %s", localFile))
	},
}

func taskLogs(sess *session.Session, task *ecs.Task) error {
	ecsSvc := ecs.New(sess)
	taskDefn, err := ecsSvc.DescribeTaskDefinition(&ecs.DescribeTaskDefinitionInput{
		TaskDefinition: task.TaskDefinitionArn,
	})
	if err != nil {
		return err
	}
	updatedTaskResp, err := ecsSvc.DescribeTasks(&ecs.DescribeTasksInput{
		Cluster: task.ClusterArn,
		Tasks:   []*string{task.TaskArn},
	})
	if err != nil {
		return err
	}
	task = updatedTaskResp.Tasks[0]

	containerDefn := taskDefn.TaskDefinition.ContainerDefinitions[0]
	logConfig := containerDefn.LogConfiguration
	taskArnParts := strings.Split(*task.TaskArn, "/")
	taskID := taskArnParts[len(taskArnParts)-1]
	sawConfig.Group = *logConfig.Options["awslogs-group"]
	sawConfig.Start = task.StartedAt.Format(time.RFC3339)
	sawConfig.Streams = []*cloudwatchlogs.LogStream{{
		LogStreamName: aws.String(
			fmt.Sprintf("%s/%s/%s",
				*logConfig.Options["awslogs-stream-prefix"],
				*containerDefn.Name,
				taskID),
		),
	}}
	newBlade(sess).GetEvents()
	return nil
}

// dbLoadCmd represents the db load command
var dbLoadCmd = &cobra.Command{
	Use:   "load <dumpfile>",
	Short: "load a dump file into the remote database",
	Long: `The dump file can either be local (in which case it will first be uploaded to S3. Or you can specify a file already on S3 by using "s3://..." as the first argument.
	
WARNING: This is a destructive action which will delete the contents of your remote database in order to load the dump in.
	`,
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		var remoteFile string
		startSpinner()
		app, err := app.Init(AppName)
		checkErr(err)
		if strings.HasPrefix(args[0], "s3://") {
			remoteFile = args[0]
		} else {
			file, err := os.Open(args[0])
			checkErr(err)
			getObjectInput, err := app.DBDumpLocation("uploads/")
			checkErr(err)
			remoteFile = fmt.Sprintf("s3://%s/%s", *getObjectInput.Bucket, *getObjectInput.Key)
			Spinner.Suffix = fmt.Sprintf(" uploading %s", args[0])
			err = uploadFile(app.Session, &s3manager.UploadInput{
				Bucket: getObjectInput.Bucket,
				Key:    getObjectInput.Key,
				Body:   file,
			})
		}
		family, err := app.DBDumpLoadFamily()
		checkErr(err)
		task, err := app.StartTask(
			family,
			[]string{"load-from-s3.sh", remoteFile},
			true,
		)
		Spinner.Suffix = fmt.Sprintf(" loading database %s", aurora.Faint(*task.TaskArn))
		checkErr(err)
		exitCode, err := app.WaitForTaskStopped(task)
		checkErr(err)
		Spinner.Stop()
		// pg_restore can have inconsequential errors... don't assume failure, but notify user
		if *exitCode != 0 && strings.Contains(app.Settings.DBUtils.Engine, "postgres") {
			taskLogs(app.Session, task)
			printWarning("check pg_restore output")
		} else if *exitCode != 0 {
			taskLogs(app.Session, task)
			printError("database load failed")
		} else {
			printSuccess(fmt.Sprintf("loaded database dump from %s", args[0]))
		}
	},
}

func init() {
	rootCmd.AddCommand(dbCmd)

	dbCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	dbCmd.MarkPersistentFlagRequired("app-name")
	dbCmd.AddCommand(dbShellCmd)
	dbCmd.AddCommand(dbDumpCmd)
	dbCmd.AddCommand(dbLoadCmd)
}
