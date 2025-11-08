/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

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
	"sort"
	"strings"

	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/apppackio/apppack/utils"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/juju/ansiterm/tabwriter"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type stackHumanize struct {
	Name    string
	Type    string
	Cluster string
}

func stackName(stack *cloudformation.Stack) (*stackHumanize, error) {
	parts := strings.Split(*stack.StackName, "-")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid stack name %s", *stack.StackName)
	}

	humanStack := stackHumanize{
		Name: strings.Join(parts[2:], "-"),
		Type: parts[1],
	}
	if humanStack.Type == "database" || humanStack.Type == "redis" || humanStack.Type == "app" || humanStack.Type == "pipeline" {
		clusterStack, err := bridge.GetStackParameter(stack.Parameters, "ClusterStackName")
		if err != nil {
			return nil, err
		}

		stackParts := strings.Split(*clusterStack, "-")
		if len(stackParts) < 3 {
			return nil, fmt.Errorf("invalid cluster stack name %s", *clusterStack)
		}

		humanStack.Cluster = strings.Join(stackParts[2:], "-")
	}

	return &humanStack, nil
}

// stacksCmd represents the stacks command
var stacksCmd = &cobra.Command{
	Use:                   "stacks",
	Short:                 "list the stacks installed at AWS",
	Long:                  "*Requires admin permissions.*",
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		ui.StartSpinner()
		sess, err := adminSession(SessionDurationSeconds)
		checkErr(err)
		stacks, err := bridge.ApppackStacks(sess)
		checkErr(err)
		sort.Slice(stacks, func(i, j int) bool {
			return *stacks[i].StackName < *stacks[j].StackName
		})
		ui.Spinner.Stop()
		currentGroup := ""
		w := new(tabwriter.Writer)
		// minwidth, tabwidth, padding, padchar, flags
		w.Init(os.Stdout, 8, 8, 0, '\t', 0)
		for _, stack := range stacks {
			if *stack.StackName == "apppack-account" || strings.HasPrefix(*stack.StackName, "apppack-region-") {
				continue
			}
			humanStack, err := stackName(stack)
			checkErr(err)
			if currentGroup != humanStack.Type {
				w.Flush()
				currentGroup = humanStack.Type
				fmt.Println()
				caser := cases.Title(language.English)
				ui.PrintHeaderln(caser.String(currentGroup + " Stacks"))
				if humanStack.Cluster != "" {
					fmt.Fprintf(w, "%s\t%s\t\n", aurora.Faint("Name"), aurora.Faint("Cluster"))
				} else {
					fmt.Fprintf(w, "%s\t\n", aurora.Faint("Name"))
				}
			}

			fmt.Fprint(w, humanStack.Name)
			if humanStack.Cluster != "" {
				fmt.Fprintf(w, "\t%s\t\n", humanStack.Cluster)
			} else {
				fmt.Fprintf(w, "\t\n")
			}
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(stacksCmd)
	stacksCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", utils.AccountFlagHelpText)
	stacksCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
}
