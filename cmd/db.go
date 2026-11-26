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
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/ui"
	"github.com/apppackio/saw/blade"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	dbFrom       string
	dbOutputFile string
	dbNoDownload bool
)

// Each of these names just the one method its caller uses, so the transfer
// manager client already satisfies them and a test fake is a plain struct with
// one method.
type (
	objectDownloader interface {
		DownloadObject(context.Context, *transfermanager.DownloadObjectInput, ...func(*transfermanager.Options)) (*transfermanager.DownloadObjectOutput, error)
	}

	objectUploader interface {
		UploadObject(context.Context, *transfermanager.UploadObjectInput, ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error)
	}
)

func downloadFile(cfg aws.Config, objInput *s3.GetObjectInput, outputFile string) error {
	return downloadObject(transfermanager.New(s3.NewFromConfig(cfg)), objInput, outputFile)
}

func downloadObject(s3Svc objectDownloader, objInput *s3.GetObjectInput, outputFile string) error {
	ui.Spinner.Suffix = " downloading " + outputFile

	// #nosec G304 -- outputFile is the destination the user asked for
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = s3Svc.DownloadObject(context.Background(), &transfermanager.DownloadObjectInput{
		Bucket:   objInput.Bucket,
		Key:      objInput.Key,
		WriterAt: file,
	})
	if err != nil {
		return err
	}

	return nil
}

func uploadFile(cfg aws.Config, uploadInput *transfermanager.UploadObjectInput) error {
	return uploadObject(transfermanager.New(s3.NewFromConfig(cfg)), uploadInput)
}

func uploadObject(s3Svc objectUploader, uploadInput *transfermanager.UploadObjectInput) error {
	_, err := s3Svc.UploadObject(context.Background(), uploadInput)
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
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		family, exec, err := a.DBShellTaskInfo()
		checkErr(err)
		StartInteractiveShell(a, family, aws.String("entrypoint.sh "+*exec), []string{"/bin/sh", "-c"}, &ecstypes.TaskOverride{})
	},
}

// progress prints a status line. --json prints one JSON document, so it is
// silent there.
func progress(text string) {
	if !AsJSON {
		fmt.Println(aurora.Faint(text))
	}
}

func s3URL(obj *s3.GetObjectInput) string {
	return fmt.Sprintf("s3://%s/%s", *obj.Bucket, *obj.Key)
}

// dumpToS3 runs the dump task for a and waits for it. It exits non-zero on
// failure, so a script never mistakes a failure for a location.
func dumpToS3(a *app.App) *s3.GetObjectInput {
	task, obj, err := a.DBDump()
	checkErr(err)
	ui.Spinner.Stop()
	progress("starting task " + *task.TaskArn)
	ui.StartSpinner()
	ui.Spinner.Suffix = " dumping " + a.Name + " database"
	exitCode, err := a.WaitForTaskStopped(task)
	checkErr(err)
	ui.Spinner.Stop()
	if *exitCode != 0 {
		_ = taskLogs(a.Session, task)
		checkErr(errors.New("database dump failed"))
	}

	return obj
}

// dumpJSON is the `db dump --json` output.
type dumpJSON struct {
	S3URL string `json:"s3_url"`
	Key   string `json:"key"`
	File  string `json:"file,omitempty"`
}

// dbDumpCmd represents the db load command
var dbDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "dump the database to a local file",
	Long: "Dump the database to `<app-name>.dump` in the current directory.\n\n" +
		"With --no-download the dump stays in S3 and its location is printed on the last line, " +
		"ready to pass to `db load`. " +
		"With --json the location is printed as JSON and nothing else.",
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		// db dump load can be really slow, let people open longer sessions to wait for it to finish
		app, err := app.Init(AppName, UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		err = app.LoadSettings()
		checkErr(err)
		if dbOutputFile != "" {
			dir, _ := filepath.Split(dbOutputFile)
			if dir != "" {
				checkErr(os.MkdirAll(dir, 0o750))
			}
		}
		getObjectInput := dumpToS3(app)
		result := dumpJSON{S3URL: s3URL(getObjectInput), Key: *getObjectInput.Key}
		if !dbNoDownload {
			if dbOutputFile == "" {
				if strings.HasSuffix(*getObjectInput.Key, ".sql.gz") {
					dbOutputFile = app.Name + ".sql.gz"
				} else {
					dbOutputFile = app.Name + ".dump"
				}
			}
			progress("dump stored at " + result.S3URL)
			ui.StartSpinner()
			err = downloadFile(app.Session, getObjectInput, dbOutputFile)
			checkErr(err)
			ui.Spinner.Stop()
			result.File = dbOutputFile
		}
		switch {
		case AsJSON:
			checkErr(printJSON(result))
		case dbNoDownload:
			printSuccess("Dumped database to S3")
			// Plain and last, so a script can take it with `tail -1`.
			fmt.Println(result.S3URL)
		default:
			printSuccess("Dumped database to " + dbOutputFile)
		}
	},
}

// dumpListJSON is one row of the `db dumps --json` output.
type dumpListJSON struct {
	Key          string    `json:"key"`
	S3URL        string    `json:"s3_url"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// dbDumpsCmd represents the db dumps command
var dbDumpsCmd = &cobra.Command{
	Use:   "dumps",
	Short: "list the database dumps stored in S3",
	Long: "List the dumps in the app's dump bucket, newest first: those made by `db dump` (dumps/), " +
		"those uploaded by `db load` (uploads/) and any others.\n\n" +
		"A key from this list can be loaded into another app with `db load --from-app <app>:<key>`.",
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		checkErr(a.LoadSettings())
		bucket := a.Settings.DBUtils.S3Bucket
		dumps := []dumpListJSON{}
		pages := s3.NewListObjectsV2Paginator(s3.NewFromConfig(a.Session), &s3.ListObjectsV2Input{Bucket: &bucket})
		for pages.HasMorePages() {
			page, err := pages.NextPage(context.Background())
			checkErr(err)
			for _, obj := range page.Contents {
				dumps = append(dumps, dumpListJSON{
					Key:          *obj.Key,
					S3URL:        fmt.Sprintf("s3://%s/%s", bucket, *obj.Key),
					Size:         aws.ToInt64(obj.Size),
					LastModified: aws.ToTime(obj.LastModified),
				})
			}
		}
		sort.Slice(dumps, func(i, j int) bool { return dumps[i].LastModified.After(dumps[j].LastModified) })
		ui.Spinner.Stop()
		if AsJSON {
			checkErr(printJSON(dumps))

			return
		}
		if len(dumps) == 0 {
			fmt.Println(aurora.Faint("no dumps in s3://" + bucket))

			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		// write errors surface in Flush
		_, _ = fmt.Fprintln(w, "MODIFIED\tSIZE\tKEY")
		for _, d := range dumps {
			_, _ = fmt.Fprintf(w, "%s\t%d MB\t%s\n", d.LastModified.Local().Format("2006-01-02 15:04"), d.Size>>20, d.Key)
		}
		checkErr(w.Flush())
	},
}

func taskLogs(cfg aws.Config, task *ecstypes.Task) error {
	ecsSvc := ecs.NewFromConfig(cfg)

	taskDefn, err := ecsSvc.DescribeTaskDefinition(context.Background(), &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: task.TaskDefinitionArn,
	})
	if err != nil {
		return err
	}

	updatedTaskResp, err := ecsSvc.DescribeTasks(context.Background(), &ecs.DescribeTasksInput{
		Cluster: task.ClusterArn,
		Tasks:   []string{*task.TaskArn},
	})
	if err != nil {
		return err
	}

	task = &updatedTaskResp.Tasks[0]

	containerDefn := taskDefn.TaskDefinition.ContainerDefinitions[0]
	logConfig := containerDefn.LogConfiguration
	taskArnParts := strings.Split(*task.TaskArn, "/")
	taskID := taskArnParts[len(taskArnParts)-1]
	sawConfig.Group = logConfig.Options["awslogs-group"]
	sawConfig.Start = task.StartedAt.Format(time.RFC3339)
	// Filter by stream prefix rather than naming the stream outright. The
	// prefix identifies exactly one stream here, so the two are equivalent.
	sawConfig.Prefix = fmt.Sprintf("%s/%s/%s",
		logConfig.Options["awslogs-stream-prefix"],
		*containerDefn.Name,
		taskID)

	blade.NewBladeWithConfig(cfg, &sawConfig, &sawOutputConfig).GetEvents()

	return nil
}

var postgresLoadJobs int

func flagIsSet(flags *pflag.FlagSet, name string) bool {
	found := false

	flags.Visit(func(flag *pflag.Flag) {
		if flag.Name == name {
			found = true
		}
	})

	return found
}

// accountOf returns the AWS account an app runs in, read from its cluster ARN
// (arn:aws:ecs:<region>:<account>:cluster/<name>).
func accountOf(a *app.App) string {
	if parts := strings.Split(a.Settings.Cluster.ARN, ":"); len(parts) > 4 {
		return parts[4]
	}

	return ""
}

// copyFailedExit is the load task's exit status when --from-app could not copy
// the dump into the target's bucket, which happens before anything is dropped.
const copyFailedExit = 64

// fromRe splits --from-app into an app and an optional dump: "app", "app:dump",
// "pipeline:12" (a review app) or "pipeline:12:dump". A dump name made only
// of digits cannot be told from a review app; give its full name.
var fromRe = regexp.MustCompile(`^([^:]+(?::\d+)?)(?::(.+))?$`)

func parseFrom(from string) (appName, dump string) {
	m := fromRe.FindStringSubmatch(from)
	if m == nil {
		return from, ""
	}

	return m[1], m[2]
}

// dbLoadCmd represents the db load command
var dbLoadCmd = &cobra.Command{
	Use:   "load [<dumpfile>]",
	Short: "load a dump file into the remote database",
	Long: `The dump file can either be local (in which case it will first be uploaded to S3. Or you can specify a file already on S3 by using "s3://..." as the first argument.

With --from-app <app> the database of another app is dumped and loaded in one step, without passing through this machine. Postgres only.
Use --from-app <app>:<dump> to reuse an existing dump in that app's bucket instead of taking a new one, e.g. --from-app my-app:weekly.

WARNING: This is a destructive action which will delete the contents of your remote database in order to load the dump in.
	`,
	Args: func(cmd *cobra.Command, args []string) error {
		if dbFrom != "" {
			return cobra.NoArgs(cmd, args)
		}

		return cobra.ExactArgs(1)(cmd, args)
	},
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		var remoteFile string
		initApp := app.Init // the local `app` below shadows the package
		fromApp, fromDump := parseFrom(dbFrom)
		// before anything else: copying an app onto itself would only destroy it
		if fromApp == AppName {
			checkErr(fmt.Errorf("--from-app and --app-name are both %s; they must be different apps", AppName))
		}
		loaded := fromApp + " database into " + AppName
		if dbFrom == "" {
			loaded = "database dump from " + args[0]
		}
		ui.StartSpinner()
		// db dump load can be really slow, let people open longer sessions to wait for it to finish
		app, err := initApp(AppName, UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		checkErr(app.LoadSettings())
		isPostgres := strings.Contains(app.Settings.DBUtils.Engine, "postgres")
		source := app
		if dbFrom != "" {
			source, err = initApp(fromApp, UseAWSCredentials, MaxSessionDurationSeconds)
			checkErr(err)
			checkErr(source.LoadSettings())
			// Having access to both apps is not enough: someone who works across
			// customers could copy one tenant's data into another's account.
			if src, dst := accountOf(source), accountOf(app); src != dst {
				checkErr(fmt.Errorf("%s is in AWS account %s and %s is in %s; --from-app only copies within one account", fromApp, src, AppName, dst))
			}
			// ponytail: Postgres only; the MySQL db-utils image has no curl
			if !isPostgres || !strings.Contains(source.Settings.DBUtils.Engine, "postgres") {
				checkErr(errors.New("--from-app is only supported between Postgres databases"))
			}
		}
		// exit if we're not using postgres and --jobs is set
		if !isPostgres && flagIsSet(cmd.Flags(), "jobs") {
			checkErr(errors.New("the --jobs/-j flag is only supported for Postgres databases"))
		}
		if postgresLoadJobs < 1 {
			checkErr(errors.New("the --jobs/-j flag must be set to a positive integer"))
		}
		family, err := app.DBDumpLoadFamily()
		checkErr(err)
		ui.Spinner.Stop()
		warning := "This will destroy any data that is currently in the database."
		if dbFrom != "" {
			// -a is destroyed and --from-app is copied; say which is which
			warning = fmt.Sprintf("This replaces %s's database with a copy of %s's.", AppName, fromApp)
		}
		confirmAction(warning, AppName)
		ui.StartSpinner()
		command := []string{"load-from-s3.sh"}
		var env []ecstypes.KeyValuePair
		if dbFrom != "" {
			var dump *s3.GetObjectInput
			if fromDump != "" {
				// An existing dump: its name in the source app's dump bucket, with or
				// without the dumps/ prefix and extension.
				bucket := source.Settings.DBUtils.S3Bucket
				var head *s3.HeadObjectOutput
				for _, key := range []string{fromDump, "dumps/" + fromDump, fromDump + ".dump", "dumps/" + fromDump + ".dump"} {
					head, err = s3.NewFromConfig(source.Session).HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: &bucket, Key: &key})
					if err == nil {
						dump = &s3.GetObjectInput{Bucket: &bucket, Key: &key}

						break
					}
				}
				if dump == nil {
					checkErr(fmt.Errorf("cannot read dump %q in %s's dump bucket %s: %w", fromDump, fromApp, bucket, err))
				}
				ui.Spinner.Stop()
				progress(fmt.Sprintf("using existing dump %s (%d MB, from %s)", s3URL(dump), aws.ToInt64(head.ContentLength)>>20, aws.ToTime(head.LastModified).Format(time.RFC3339)))
			} else {
				dump = dumpToS3(source)
				progress("dump stored at " + s3URL(dump))
			}
			// The load task cannot read the source app's bucket, so it fetches
			// the dump through a short-lived link into its own bucket first.
			// It is signed with the source session, so it also dies with that session.
			presigned, err := s3.NewPresignClient(s3.NewFromConfig(source.Session)).PresignGetObject(
				context.Background(), dump, s3.WithPresignExpires(5*time.Minute),
			)
			checkErr(err)
			dest, err := app.DBDumpLocation("uploads/")
			checkErr(err)
			remoteFile = s3URL(dest)
			progress("copying to " + remoteFile)
			env = append(env, ecstypes.KeyValuePair{Name: aws.String("DUMP_URL"), Value: &presigned.URL})
			// pipefail: a failed download must not reach the load, which drops the database first
			// ponytail: streams through `aws s3 cp -`, which needs --expected-size above ~50GB
			command = []string{
				"bash", "-c",
				`set -euo pipefail; curl -fsSL "$DUMP_URL" | aws s3 cp --no-progress - "$0" || exit ` + strconv.Itoa(copyFailedExit) + `; exec load-from-s3.sh "$0"`,
			}
			ui.StartSpinner()
		} else if strings.HasPrefix(args[0], "s3://") {
			remoteFile = args[0]
		} else {
			file, err := os.Open(args[0])
			checkErr(err)
			getObjectInput, err := app.DBDumpLocation("uploads/")
			checkErr(err)
			remoteFile = fmt.Sprintf("s3://%s/%s", *getObjectInput.Bucket, *getObjectInput.Key)
			ui.Spinner.Suffix = " uploading " + args[0]
			err = uploadFile(app.Session, &transfermanager.UploadObjectInput{
				Bucket: getObjectInput.Bucket,
				Key:    getObjectInput.Key,
				Body:   file,
			})
			checkErr(err)
		}
		taskOverride := &ecstypes.TaskOverride{}
		if isPostgres {
			env = append(env, ecstypes.KeyValuePair{Name: aws.String("PG_RESTORE_JOBS"), Value: aws.String(strconv.Itoa(postgresLoadJobs))})
		}
		if len(env) > 0 {
			taskOverride.ContainerOverrides = []ecstypes.ContainerOverride{{Name: aws.String("app"), Environment: env}}
		}
		task, err := app.StartTask(
			family,
			append(command, remoteFile),
			taskOverride,
			true,
		)
		checkErr(err)
		ui.Spinner.Stop()
		fmt.Println(aurora.Faint("starting task " + *task.TaskArn))
		ui.StartSpinner()
		ui.Spinner.Suffix = " loading database"
		exitCode, err := app.WaitForTaskStopped(task)
		checkErr(err)
		ui.Spinner.Stop()
		if *exitCode == 0 {
			printSuccess("loaded " + loaded)

			return
		}
		_ = taskLogs(app.Session, task)
		switch {
		case dbFrom != "" && *exitCode == copyFailedExit:
			printError("could not copy the dump; the database was not touched")
		case isPostgres:
			// pg_restore can have inconsequential errors... don't assume the data is bad, but notify user
			printWarning("the load task reported errors; check the output above (some pg_restore errors are harmless)")
		default:
			printError("database load failed")
		}
		// a failed load used to exit 0, which hid it from scripts
		os.Exit(1)
	},
}

func init() {
	rootCmd.AddCommand(dbCmd)

	dbCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	_ = dbCmd.MarkPersistentFlagRequired("app-name")
	dbCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
	dbCmd.AddCommand(dbShellCmd)
	dbCmd.AddCommand(dbDumpCmd)
	dbDumpCmd.Flags().StringVarP(&dbOutputFile, "output", "o", "", "path to output file -- default will be <app-name> with the appropriate extension for the database")
	dbDumpCmd.Flags().BoolVar(&dbNoDownload, "no-download", false, "leave the dump in S3 and print its location instead of downloading it")
	dbDumpCmd.MarkFlagsMutuallyExclusive("output", "no-download")
	dbCmd.AddCommand(dbDumpsCmd)
	dbCmd.AddCommand(dbLoadCmd)
	dbLoadCmd.Flags().StringVar(&dbFrom, "from-app", "", "load another app's database instead of a file: <app> for a fresh dump, <app>:<dump> for an existing one")
	dbLoadCmd.Flags().IntVarP(&postgresLoadJobs, "jobs", "j", 2, "number of jobs to use for the load (passed through as --jobs to pg_restore -- Postgres only)")
}
