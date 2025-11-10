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

	"github.com/apppackio/apppack/stacks"
	"github.com/apppackio/apppack/ui"
	"github.com/apppackio/apppack/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func modifyAppStack(cfg aws.Config, stack *stacks.AppStack, name string, flags *pflag.FlagSet) error {
	// Track which flags were actually provided
	providedFlags := make(map[string]bool)
	flags.Visit(func(f *pflag.Flag) {
		providedFlags[f.Name] = true
	})

	// If no modifiable flags provided, run interactive mode
	hasModifiableFlags := providedFlags["repository"] || providedFlags["branch"] ||
		providedFlags["domains"] || providedFlags["healthcheck-path"]

	if !hasModifiableFlags {
		// Interactive mode - ask questions with current values as defaults
		if err := askModifyQuestions(cfg, stack); err != nil {
			return err
		}
	} else {
		// Flag mode - update only provided values
		if providedFlags["repository"] {
			repo, _ := flags.GetString("repository")
			stack.Parameters.RepositoryURL = repo
			if err := stack.Parameters.SetRepositoryType(); err != nil {
				return err
			}
		}
		if providedFlags["branch"] && !stack.Pipeline {
			branch, _ := flags.GetString("branch")
			stack.Parameters.Branch = branch
		}
		if providedFlags["domains"] && !stack.Pipeline {
			domains, _ := flags.GetStringSlice("domains")
			stack.Parameters.Domains = domains
		}
		if providedFlags["healthcheck-path"] {
			path, _ := flags.GetString("healthcheck-path")
			stack.Parameters.HealthCheckPath = path
		}
	}

	// Verify repository credentials if repository was changed
	if providedFlags["repository"] || !hasModifiableFlags {
		if err := stacks.VerifySourceCredentials(cfg, stack.Parameters.RepositoryType); err != nil {
			return err
		}
	}

	// Apply the changes
	ui.StartSpinner()
	if err := stacks.ModifyStack(cfg, stack, &name); err != nil {
		ui.Spinner.Stop()
		return err
	}

	ui.Spinner.Stop()
	ui.PrintSuccess(fmt.Sprintf("modified app %s", name))

	return nil
}

func askModifyQuestions(cfg aws.Config, stack *stacks.AppStack) error {
	var questions []*ui.QuestionExtra

	// Repository URL
	questions = append(questions, stacks.BuildRepositoryURLQuestion(&stack.Parameters.RepositoryURL))

	if err := ui.AskQuestions(questions, stack.Parameters); err != nil {
		return err
	}

	if err := stack.Parameters.SetRepositoryType(); err != nil {
		return err
	}

	questions = []*ui.QuestionExtra{}

	// Branch and Domains (only for non-pipeline apps)
	if !stack.Pipeline {
		questions = append(questions, stacks.BuildBranchQuestion(&stack.Parameters.Branch))
		questions = append(questions, stacks.BuildDomainsQuestion(&stack.Parameters.Domains))
	}

	// Healthcheck path
	questions = append(questions, stacks.BuildHealthCheckPathQuestion(&stack.Parameters.HealthCheckPath))

	return ui.AskQuestions(questions, stack.Parameters)
}

// modifyCmd represents the modify command
var modifyCmd = &cobra.Command{
	Use:   "modify",
	Short: "modify AppPack resources",
	Long: `Use subcommands to modify AppPack resources.

These require administrator access.
`,
	DisableFlagsInUseLine: true,
}

// modifyAppCmd represents the modify app command
var modifyAppCmd = &cobra.Command{
	Use:   "app <name>",
	Short: "modify an AppPack application",
	Long: `*Requires admin permissions.*

Modify an existing AppPack application or pipeline.

If no flags are provided, an interactive prompt will be provided.`,
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Example: `  # Interactive mode - prompts for all modifiable parameters
  apppack modify app my-app

  # Update specific parameters with flags
  apppack modify app my-app --branch develop
  apppack modify app my-app --repository https://github.com/org/new-repo.git --branch main
  apppack modify app my-app --domains example.com,www.example.com`,
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		// Prevent modifying review apps - they should be managed at the pipeline level
		if stacks.IsReviewAppName(name) {
			checkErr(fmt.Errorf("cannot modify review app directly. Review apps are managed at the pipeline level. To change settings, modify the pipeline instead"))
		}

		ui.StartSpinner()
		cfg, err := adminSession(SessionDurationSeconds)
		checkErr(err)

		stack, err := appOrPipelineStack(cfg, name)
		checkErr(err)

		ui.Spinner.Stop()
		fmt.Print(aurora.Green(fmt.Sprintf("🔧 Modifying %s `%s`", stack.StackType(), name)).String())
		if CurrentAccountRole != nil {
			fmt.Print(aurora.Green(" on " + CurrentAccountRole.GetAccountName()).String())
		}
		fmt.Println()

		checkErr(modifyAppStack(cfg, stack, name, cmd.Flags()))
	},
}

func init() {
	rootCmd.AddCommand(modifyCmd)

	modifyCmd.AddCommand(modifyAppCmd)
	modifyAppCmd.Flags().StringP("repository", "r", "", "repository URL")
	modifyAppCmd.Flags().StringP("branch", "b", "", "branch name")
	modifyAppCmd.Flags().StringSliceP("domains", "d", []string{}, "custom domains (comma-separated)")
	modifyAppCmd.Flags().String("healthcheck-path", "", "healthcheck path")
	modifyAppCmd.Flags().StringVarP(
		&AccountIDorAlias,
		"account",
		"c",
		"",
		utils.AccountFlagHelpText,
	)
}
