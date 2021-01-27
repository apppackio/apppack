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

	"github.com/spf13/cobra"
)

// accountSetupCmd represents the accountSetup command
var accountSetupCmd = &cobra.Command{
	Use:   "account-setup",
	Short: "setup your AppPack account and create initial resources",
	Long:  "*Requires AWS credentials.*\n\nThis is a shortcut for `apppack create account && apppack create region`",
	Run: func(cmd *cobra.Command, args []string) {
		accountCmd.Run(cmd, []string{})
		fmt.Println("")
		createRegionCmd.Run(cmd, []string{})
	},
}

func init() {
	rootCmd.AddCommand(accountSetupCmd)
	accountSetupCmd.Flags().StringP("dockerhub-username", "u", "", "Docker Hub username")
	accountSetupCmd.Flags().StringP("dockerhub-access-token", "t", "", "Docker Hub Access Token (https://hub.docker.com/settings/security)")
	accountSetupCmd.Flags().StringVar(&region, "region", "", "AWS region to create resources in")
}
