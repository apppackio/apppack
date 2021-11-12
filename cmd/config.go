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
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/ansiterm/tabwriter"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ssm"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/bridge"
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
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		svc := ssm.New(a.Session)
		resp, err := svc.GetParameter(&ssm.GetParameterInput{
			Name:           aws.String(fmt.Sprintf("%s%s", a.ConfigPrefix(), args[0])),
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
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		err = a.SetConfig(name, value, true)
		checkErr(err)
		Spinner.Stop()
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
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		svc := ssm.New(a.Session)
		_, err = svc.DeleteParameter(&ssm.DeleteParameterInput{
			Name: aws.String(fmt.Sprintf("%s%s", a.ConfigPrefix(), args[0])),
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
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		parameters, err := a.GetConfig()
		checkErr(err)
		bridge.SortParameters(parameters)
		Spinner.Stop()
		for _, value := range parameters {
			parts := strings.Split(*value.Name, "/")
			varname := parts[len(parts)-1]
			fmt.Fprintf(w, "%s\t%s\t\n", aurora.Green(fmt.Sprintf("%s:", varname)), *value.Value)
		}
		fmt.Println(aurora.Faint("==="), aurora.Bold(aurora.White(fmt.Sprintf("%s Config Vars", AppName))))
		w.Flush()
		if a.IsReviewApp() {
			fmt.Println()
			a.ReviewApp = nil
			parameters, err := a.GetConfig()
			checkErr(err)
			bridge.SortParameters(parameters)
			Spinner.Stop()
			for _, value := range parameters {
				parts := strings.Split(*value.Name, "/")
				varname := parts[len(parts)-1]
				fmt.Fprintf(w, "%s\t%s\t\n", aurora.Green(fmt.Sprintf("%s:", varname)), *value.Value)
			}
			fmt.Println(aurora.Faint("==="), aurora.Bold(aurora.White(fmt.Sprintf("%s Config Vars (inherited)", a.Name))))
			w.Flush()
		}
	},
}

var includeManagedVars bool

// parameterIsManaged checks is the parameter was created by a Cloudformation stack
func parameterIsManaged(ssmSvc *ssm.SSM, parameter *ssm.Parameter) (*bool, error) {
	resp, err := ssmSvc.ListTagsForResource(&ssm.ListTagsForResourceInput{
		ResourceId:   parameter.Name,
		ResourceType: aws.String(ssm.ResourceTypeForTaggingParameter),
	})
	if err != nil {
		return nil, err
	}
	for _, tag := range resp.TagList {
		if *tag.Key == "aws:cloudformation:stack-id" || *tag.Key == "apppack:cloudformation:stack-id" {
			return aws.Bool(true), nil
		}
	}
	return aws.Bool(false), nil
}

// configExportCmd represents the config export command
var configExportCmd = &cobra.Command{
	Use:                   "export",
	Short:                 "export the config variables to JSON",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		parameters, err := a.GetConfig()
		bridge.SortParameters(parameters)
		checkErr(err)
		config := make(map[string]string)
		ssmSvc := ssm.New(a.Session)
		for _, p := range parameters {
			parts := strings.Split(*p.Name, "/")
			varname := parts[len(parts)-1]
			if !includeManagedVars {
				isManaged, err := parameterIsManaged(ssmSvc, p)
				checkErr(err)
				if *isManaged {
					continue
				}
			}
			config[varname] = *p.Value
		}
		j, err := json.Marshal(config)
		checkErr(err)
		Spinner.Stop()
		b := bytes.NewBuffer([]byte{})
		json.Indent(b, j, "", "  ")
		fmt.Println(b.String())
	},
}

var importConfigOverride bool

// configImportCmd represents the config export command
var configImportCmd = &cobra.Command{
	Use:                   "import <file>",
	Short:                 "import config variables from a JSON file",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, SessionDurationSeconds)
		checkErr(err)
		data, err := ioutil.ReadFile(args[0])
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
				if aerr, ok := err.(awserr.Error); ok {
					if ok && aerr.Code() == "ParameterAlreadyExists" && !importConfigOverride {
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
		Spinner.Stop()
		printSuccess(msg)

	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	configCmd.MarkPersistentFlagRequired("app-name")
	configCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")

	configCmd.AddCommand(getCmd)
	configCmd.AddCommand(setCmd)
	configCmd.AddCommand(unsetCmd)
	configCmd.AddCommand(listCmd)
	configCmd.AddCommand(configExportCmd)
	configExportCmd.Flags().BoolVar(&includeManagedVars, "all", false, "include AppPack managed variables (e.g. DATABASE_URL)")

	configCmd.AddCommand(configImportCmd)
	configImportCmd.Flags().BoolVar(&importConfigOverride, "overwrite", false, "overwrite variables if they already exist")
}
