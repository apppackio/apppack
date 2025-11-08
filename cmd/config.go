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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/juju/ansiterm"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:                   "config",
	Short:                 "manage app configuration (environment variables/secrets)",
	Long:                  `Configuration is stored in SSM Parameter Store and injected into the application containers at runtime.`,
	DisableFlagsInUseLine: true,
}

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:                   "get <variable>",
	Short:                 "show the value of a single config variable",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		svc := ssm.New(a.Session)
		resp, err := svc.GetParameter(&ssm.GetParameterInput{
			Name:           aws.String(fmt.Sprintf("%s%s", a.ConfigPrefix(), args[0])),
			WithDecryption: aws.Bool(true),
		})
		ui.Spinner.Stop()
		checkErr(err)
		fmt.Println(*resp.Parameter.Value)
	},
}

// setCmd represents the config set command
var setCmd = &cobra.Command{
	Use:                   "set <variable>=<value>",
	Short:                 "set the value of a single config variable",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Example:               "apppack -a my-app config set ENVIRONMENT=production",
	Run: func(_ *cobra.Command, args []string) {
		if !strings.Contains(args[0], "=") {
			checkErr(errors.New("argument should be in the form <variable>=<value>"))
		}
		parts := strings.SplitN(args[0], "=", 2)
		name := parts[0]
		value := parts[1]
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		err = a.SetConfig(name, value, true)
		checkErr(err)
		ui.Spinner.Stop()
		printSuccess("stored config variable " + name)
	},
}

// unsetCmd represents the get command
var unsetCmd = &cobra.Command{
	Use:                   "unset <variable>",
	Short:                 "remove a config variable",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		name := args[0]
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		svc := ssm.New(a.Session)
		_, err = svc.DeleteParameter(&ssm.DeleteParameterInput{
			Name: aws.String(fmt.Sprintf("%s%s", a.ConfigPrefix(), args[0])),
		})
		ui.Spinner.Stop()
		checkErr(err)
		printSuccess("removed config variable " + name)
	},
}

// configListCmd represents the list command
var configListCmd = &cobra.Command{
	Use:                   "list",
	Short:                 "list all config variables and values",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(0),
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		configVars, err := a.GetConfig()
		checkErr(err)
		ui.Spinner.Stop()

		if AsJSON {
			buf, err := configVars.ToJSON()
			checkErr(err)
			fmt.Println(buf.String())

			return
		}

		// minwidth, tabwidth, padding, padchar, flags
		w := ansiterm.NewTabWriter(os.Stdout, 8, 8, 0, '\t', 0)

		if isatty.IsTerminal(os.Stdout.Fd()) {
			w.SetColorCapable(true)
		}

		ui.PrintHeaderln(AppName + " Config Vars")
		configVars.ToConsole(w)
		checkErr(w.Flush())

		if a.IsReviewApp() {
			fmt.Println()
			a.ReviewApp = nil
			ui.StartSpinner()
			parameters, err := a.GetConfig()
			checkErr(err)
			ui.Spinner.Stop()
			parameters.ToConsole(w)
			ui.PrintHeaderln(a.Name + " Config Vars (inherited)")
			checkErr(w.Flush())
		}
	},
}

var includeManagedVars bool

// configExportCmd represents the config export command
var configExportCmd = &cobra.Command{
	Use:                   "export",
	Short:                 "export the config variables to JSON",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(0),
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		configVars, err := a.GetConfigWithManaged()
		checkErr(err)
		ui.Spinner.Stop()
		var buf *bytes.Buffer
		if includeManagedVars {
			buf, err = configVars.ToJSON()
		} else {
			buf, err = configVars.ToJSONUnmanaged()
		}
		checkErr(err)
		fmt.Println(buf.String())
	},
}

var importConfigOverride bool

// configImportCmd represents the config export command
var configImportCmd = &cobra.Command{
	Use:                   "import <file>",
	Short:                 "import config variables from a JSON file",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		data, err := os.ReadFile(args[0])
		if err != nil {
			fmt.Print(err)
		}
		config := make(map[string]string)
		err = json.Unmarshal(data, &config)
		checkErr(err)
		imported := 0
		skipped := 0
		for key, val := range config {
			err = a.SetConfig(key, val, importConfigOverride)
			if err != nil {
				var aerr awserr.Error
				if errors.As(err, &aerr) {
					if aerr.Code() == "ParameterAlreadyExists" && !importConfigOverride {
						skipped++

						continue
					}
				}
				checkErr(err)
			} else {
				imported++
			}
		}
		msg := fmt.Sprintf("imported %d variables", imported)
		if skipped > 0 {
			msg = fmt.Sprintf("%s / %d skipped", msg, skipped)
		}
		ui.Spinner.Stop()
		printSuccess(msg)
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	configCmd.MarkPersistentFlagRequired("app-name")
	configCmd.PersistentFlags().BoolVar(
		&UseAWSCredentials,
		"aws-credentials",
		false,
		"use AWS credentials instead of AppPack.io federation",
	)

	configCmd.AddCommand(getCmd)
	configCmd.AddCommand(setCmd)
	configCmd.AddCommand(unsetCmd)
	configCmd.AddCommand(configListCmd)
	configListCmd.Flags().BoolVarP(&AsJSON, "json", "j", false, "output as JSON")
	configCmd.AddCommand(configExportCmd)
	configExportCmd.Flags().BoolVar(&includeManagedVars,
		"all",
		false,
		"include AppPack managed variables (e.g. DATABASE_URL)",
	)

	configCmd.AddCommand(configImportCmd)
	configImportCmd.Flags().BoolVar(&importConfigOverride,
		"overwrite",
		false,
		"overwrite variables if they already exist",
	)
}
