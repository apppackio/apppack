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
	"github.com/spf13/cobra"
)

// modifyCmd represents the create command
var modifyCmd = &cobra.Command{
	Use:   "modify",
	Short: "modify AppPack resources in your AWS account",
	Long: `Use subcommands to modify AppPack resources in your account.
	
These require administrator access.
`,
	DisableFlagsInUseLine: true,
}

func init() {
	rootCmd.AddCommand(modifyCmd)
	modifyCmd.PersistentFlags().StringVarP(&AccountIDorAlias, "account", "c", "", "AWS account ID or alias (not needed if you are only the administrator of one account)")
	modifyCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")

	modifyCmd.AddCommand(modifyAppCmd)
}
