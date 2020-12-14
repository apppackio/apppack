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
	"net/url"
	"reflect"
	"strings"
	"unsafe"

	"github.com/TylerBrock/saw/blade"
	sawconfig "github.com/TylerBrock/saw/config"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/lincolnloop/apppack/app"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var sawConfig sawconfig.Configuration
var sawOutputConfig sawconfig.OutputConfiguration

// newBlade is a hack to get a Blade instance with our AWS session
func newBlade(session *session.Session) *blade.Blade {
	b := blade.Blade{}
	setField := func(name string, value interface{}) {
		field := reflect.ValueOf(&b).Elem().FieldByName(name)
		reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
	}
	setField("cwl", cloudwatchlogs.New(session))
	setField("config", &sawConfig)
	setField("output", &sawOutputConfig)
	return &b
}

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Access application logs from Cloudwatch Logs",
	Long:  `Access application logs from Cloudwatch Logs`,
}

// logsCmd represents the logs command
var logsViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Print application logs to terminal",
	Long:  `Print application logs to terminal`,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		a.LoadSettings()
		sawConfig.Group = a.Settings.LogGroup.Name
		sawOutputConfig.Pretty = !sawOutputConfig.Raw
		// convert to format saw eexpects
		sawConfig.Start = fmt.Sprintf("-%s", sawConfig.Start)
		if sawConfig.End != "" && sawConfig.End != "now" {
			sawConfig.End = fmt.Sprintf("-%s", sawConfig.Start)
		}
		b := newBlade(a.Session)
		if sawConfig.Prefix != "" {
			streams := b.GetLogStreams()
			if len(streams) == 0 {
				checkErr(fmt.Errorf("no streams found in %s with prefix %s", sawConfig.Group, sawConfig.Prefix))
			}
			sawConfig.Streams = streams
		}
		Spinner.Stop()
		if cmd.Flags().Lookup("follow").Value.String() == "true" {
			b.StreamEvents()
		} else {
			b.GetEvents()
		}
	},
}

// logsCmd represents the logs command
var logsConsoleCmd = &cobra.Command{
	Use:   "console",
	Short: "View logs in the AWS web console",
	Long:  `View logs in the AWS web console`,
	Run: func(cmd *cobra.Command, args []string) {
		a, err := app.Init(AppName)
		a.LoadSettings()
		checkErr(err)
		logGroupParam := strings.ReplaceAll(url.QueryEscape(a.Settings.LogGroup.Name), "%", "*")
		queryParam := strings.ReplaceAll(url.QueryEscape("fields @timestamp, @message\n| sort @timestamp desc\n| limit 200"), "%", "*")
		// TODO remove hard-coded region
		region := "us-east-1"
		destinationURL := fmt.Sprintf("https://console.aws.amazon.com/cloudwatch/home?region=%s#logsV2:logs-insights$3FqueryDetail$3D~(editorString~'%s~source~(~'%s))", region, queryParam, logGroupParam)
		signinURL, err := a.GetConsoleURL(destinationURL)
		checkErr(err)
		browser.OpenURL(*signinURL)
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "App name (required)")
	logsCmd.MarkPersistentFlagRequired("app-name")

	logsCmd.AddCommand(logsConsoleCmd)
	logsCmd.AddCommand(logsViewCmd)
	logsViewCmd.Flags().StringVar(&sawConfig.Prefix, "prefix", "", `log group prefix filter
Use this to filter logs for specific services, e.g. "web", "worker"`)
	logsViewCmd.Flags().StringVar(
		&sawConfig.Start,
		"start",
		"5m",
		`start getting the logs from this point
Takes an absolute timestamp in RFC3339 format, or a relative time (eg. 2h).
Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".`,
	)
	logsViewCmd.Flags().StringVar(
		&sawConfig.End,
		"stop",
		"now",
		`stop getting the logs at this point
Takes an absolute timestamp in RFC3339 format, or a relative time (eg. 2h).
Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".`,
	)
	logsViewCmd.Flags().StringVar(&sawConfig.Filter, "filter", "", "event filter pattern")
	logsViewCmd.Flags().BoolVar(&sawOutputConfig.Raw, "raw", false, "No timestamp, log group or colors")
	logsViewCmd.Flags().BoolVar(&sawOutputConfig.Expand, "expand", false, "indent JSON log messages")
	logsViewCmd.Flags().BoolVar(&sawOutputConfig.Invert, "invert", false, "invert colors for light terminal themes")
	logsViewCmd.Flags().BoolVar(&sawOutputConfig.RawString, "rawString", false, "print JSON strings without escaping")
	logsViewCmd.Flags().BoolP("follow", "f", false, "Stream logs to console")
}
