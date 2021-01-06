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
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"

	"github.com/lincolnloop/apppack/app"
	"github.com/logrusorgru/aurora"
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
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		svc := ssm.New(a.Session)
		resp, err := svc.GetParameter(&ssm.GetParameterInput{
			Name:           aws.String(fmt.Sprintf("/paaws/apps/%s/config/%s", AppName, args[0])),
			WithDecryption: aws.Bool(true),
		})
		Spinner.Stop()
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
	Run: func(cmd *cobra.Command, args []string) {
		if !strings.Contains(args[0], "=") {
			checkErr(fmt.Errorf("argument should be in the form <variable>=<value>"))
		}
		parts := strings.SplitN(args[0], "=", 2)
		name := parts[0]
		value := parts[1]
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		svc := ssm.New(a.Session)
		_, err = svc.PutParameter(&ssm.PutParameterInput{
			Name:      aws.String(fmt.Sprintf("/paaws/apps/%s/config/%s", AppName, name)),
			Type:      aws.String("SecureString"),
			Overwrite: aws.Bool(true),
			Value:     &value,
		})
		Spinner.Stop()
		checkErr(err)
		printSuccess(fmt.Sprintf("stored config variable %s", name))
	},
}

// unsetCmd represents the get command
var unsetCmd = &cobra.Command{
	Use:                   "unset <variable>",
	Short:                 "remove a config variable",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		name := args[0]
		a, err := app.Init(AppName)
		checkErr(err)
		svc := ssm.New(a.Session)
		_, err = svc.DeleteParameter(&ssm.DeleteParameterInput{
			Name: aws.String(fmt.Sprintf("/paaws/apps/%s/config/%s", AppName, args[0])),
		})
		Spinner.Stop()
		checkErr(err)
		printSuccess(fmt.Sprintf("removed config variable %s", name))
	},
}

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:                   "list",
	Short:                 "list all config variables and values",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		w := new(tabwriter.Writer)
		// minwidth, tabwidth, padding, padchar, flags
		w.Init(os.Stdout, 8, 8, 0, '\t', 0)
		startSpinner()
		a, err := app.Init(AppName)
		checkErr(err)
		checkErr(err)
		svc := ssm.New(a.Session)
		resp, err := svc.GetParametersByPath(&ssm.GetParametersByPathInput{
			Path:           aws.String(fmt.Sprintf("/paaws/apps/%s/config/", AppName)),
			WithDecryption: aws.Bool(true),
		})
		Spinner.Stop()
		if err != nil {
			log.Fatalf("AWS API call failed: %v\n", err)
		}
		for _, value := range resp.Parameters {
			parts := strings.Split(*value.Name, "/")
			varname := parts[len(parts)-1]
			fmt.Fprintf(w, "%s\t%s\t\n", aurora.Green(fmt.Sprintf("%s:", varname)), *value.Value)
		}
		fmt.Println(aurora.Faint("==="), aurora.Bold(aurora.White(fmt.Sprintf("%s Config Vars", AppName))))
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "App name (required)")
	configCmd.MarkPersistentFlagRequired("app-name")

	configCmd.AddCommand(getCmd)
	configCmd.AddCommand(setCmd)
	configCmd.AddCommand(unsetCmd)
	configCmd.AddCommand(listCmd)
}
