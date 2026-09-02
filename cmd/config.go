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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go"
	"github.com/juju/ansiterm"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// databaseURLConfigVar is the config variable an externally-managed database is
// reached through. Setting it is necessary but not sufficient: db utils also have
// to be enabled on the app's stack before `apppack db ...` works.
const databaseURLConfigVar = "DATABASE_URL"

// hintEnableDBUtils prints a follow-up instruction after DATABASE_URL is stored on
// an app whose db utils aren't enabled yet.
//
// Setting DATABASE_URL is the natural thing a user does when pointing an app at an
// externally-managed database (Neon, Crunchy, etc.), but on its own it does nothing
// for `apppack db shell`/`db dump`/`db load` -- those need the db-utils resources,
// which only get created when the app's stack is updated. `apppack modify app` infers
// the engine from this variable, so it's the whole of the remaining work; without a
// nudge here there is nothing to tell the user that second step exists.
//
// This is advisory only. The config variable has already been stored successfully by
// the time we're called, so every failure path is silent -- a hint is never worth
// turning a succeeded command into a failed one.
func hintEnableDBUtils(a *app.App) {
	if !shouldHintEnableDBUtils(a) {
		return
	}

	printWarning(fmt.Sprintf(
		"db commands are not enabled for %s yet -- run `apppack modify app %s` to enable "+
			"`apppack db shell`/`db dump`/`db load` against this database",
		a.Name, a.Name,
	))
}

// shouldHintEnableDBUtils is the decision half of hintEnableDBUtils, split out so the
// branches are testable without stdout capture. A settings-load failure returns false:
// we can't tell whether db utils are enabled, and guessing wrong means either nagging
// a correctly-configured app or staying quiet on a misconfigured one -- silence is the
// safer error for a purely advisory message.
func shouldHintEnableDBUtils(a *app.App) bool {
	// Review apps and pipelines can't use an external database (the CloudFormation
	// condition requires IsApp), so the hint would be dead advice.
	if a.IsReviewApp() || a.Pipeline {
		return false
	}

	if err := a.LoadSettings(); err != nil {
		return false
	}

	// A non-empty engine means db utils are already wired up, either by a managed
	// AppPack database or by a previous `modify app` for this external one.
	return a.Settings.DBUtils.Engine == ""
}

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
		svc := ssm.NewFromConfig(a.Session)
		resp, err := svc.GetParameter(context.Background(), &ssm.GetParameterInput{
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

		if name == databaseURLConfigVar {
			hintEnableDBUtils(a)
		}
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
		svc := ssm.NewFromConfig(a.Session)
		_, err = svc.DeleteParameter(context.Background(), &ssm.DeleteParameterInput{
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
		databaseURLImported := false

		for key, val := range config {
			err = a.SetConfig(key, val, importConfigOverride)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					if apiErr.ErrorCode() == "ParameterAlreadyExists" && !importConfigOverride {
						skipped++

						continue
					}
				}
				checkErr(err)
			} else {
				imported++

				if key == databaseURLConfigVar {
					databaseURLImported = true
				}
			}
		}
		msg := fmt.Sprintf("imported %d variables", imported)
		if skipped > 0 {
			msg = fmt.Sprintf("%s / %d skipped", msg, skipped)
		}
		ui.Spinner.Stop()
		printSuccess(msg)

		if databaseURLImported {
			hintEnableDBUtils(a)
		}
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
	// `config list` carried its own --json/-j long before --json was promoted to a
	// root persistent flag, so re-register it locally to keep `-j` working here.
	// The shorthand can't live on the root flag: `db load` already owns -j for
	// --jobs, and the two collide the moment cobra merges the flag sets.
	// Same variable as the root flag, so `--json` behaves identically either way.
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
