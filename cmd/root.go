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
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/briandowns/spinner"
)

const ()

var cfgFile string

// Spinner is the loading animation to use for all commands
var Spinner *spinner.Spinner = spinner.New(spinner.CharSets[14], 50*time.Millisecond)

func startSpinner() {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		Spinner.Start()
	}
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "apppack",
	Short: "A CLI interface to AppPack.io",
	Long:  `AppPack is a tool to manage applications deployed on AWS via AppPack.io`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
}

func checkErr(err error) {
	if err == nil {
		return
	}
	Spinner.Stop()
	printError(fmt.Sprintf("%v", err))
	os.Exit(1)
}

func printError(text string) {
	fmt.Println(aurora.Red(fmt.Sprintf("✖ %s", text)))
}

func printSuccess(text string) {
	fmt.Println(aurora.Green(fmt.Sprintf("✔ %s", text)))
}

func printWarning(text string) {
	fmt.Println(aurora.Yellow(fmt.Sprintf("⚠ %s", text)))
}
