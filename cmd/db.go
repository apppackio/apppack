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

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
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
		shellTask, err := a.StartTask(family, app.ShellBackgroundCommand, false)
		checkErr(err)
		Spinner.Suffix = fmt.Sprintf(" starting task %s", *shellTask.TaskArn)
		err = a.WaitForTaskRunning(shellTask)
		checkErr(err)
		Spinner.Stop()
		err = a.ConnectToTask(shellTask, aws.String(fmt.Sprintf("entrypoint.sh %s", *exec)))
		checkErr(err)
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
		err = app.WaitForTaskStopped(task)
		checkErr(err)
		localFile := fmt.Sprintf("%s.dump", app.Name)
		err = downloadFile(app.Session, getObjectInput, localFile)
		checkErr(err)
		Spinner.Stop()
		printSuccess(fmt.Sprintf("Dumped database to %s", localFile))
	},
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
		err = app.WaitForTaskStopped(task)
		checkErr(err)
		Spinner.Stop()
		printSuccess(fmt.Sprintf("Loaded database dump from %s", args[0]))
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
