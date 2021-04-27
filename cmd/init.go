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

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "setup your AppPack account and create initial resources",
	Long:  "*Requires AWS credentials.*\n\nThis is a shortcut for `apppack create account && apppack create region && apppack create cluster`",
	Run: func(cmd *cobra.Command, args []string) {
		sess, err := awsSession()
		checkErr(err)
		fmt.Print(aurora.Faint("==="), aurora.Bold(aurora.Blue("Welcome to AppPack!")), " 🎉\n\n")
		fmt.Println("This will step you through the intial AppPack setup process.")
		fmt.Println("Before getting started, make sure you've taken care of the prerequisites (https://docs.apppack.io/setup/#prerequisites).")
		fmt.Printf("This process should take less than 10 minutes. After that, you'll be ready to start installing apps on your cluster.\n\n")
		alreadyInstalled, err := hasApppackOIDC(sess)
		checkErr(err)
		if *alreadyInstalled {
			fmt.Println("It looks like you've already setup your global AppPack account resources.")
			fmt.Printf("Skipping %s\n", aurora.Bold("apppack create account"))
		} else {
			fmt.Printf("running %s...\n", aurora.White("apppack create account"))
			accountCmd.Run(cmd, []string{})
		}

		fmt.Println("")
		alreadyInstalled, err = stackExists(sess, fmt.Sprintf("apppack-region-%s", *sess.Config.Region))
		checkErr(err)
		if *alreadyInstalled {
			fmt.Printf("It looks like you've already setup the %s region resources.\n", *sess.Config.Region)
			fmt.Printf("Skipping %s\n", aurora.Bold("apppack create region"))
		} else {
			fmt.Printf("running %s...\n", aurora.White("apppack create region"))
			createRegionCmd.Run(cmd, []string{})
		}

		fmt.Println("")
		clusterName := cmd.Flags().Lookup("cluster-name").Value.String()
		alreadyInstalled, err = stackExists(sess, fmt.Sprintf("apppack-cluster-%s", clusterName))
		checkErr(err)
		if *alreadyInstalled {
			fmt.Printf("It looks like you've already setup a cluster named %s.\n", clusterName)
			fmt.Printf("Skipping %s\n", aurora.Bold(fmt.Sprintf("apppack create cluster %s", clusterName)))
		} else {
			fmt.Printf("running %s...\n", aurora.White(fmt.Sprintf("apppack create cluster %s", clusterName)))
			createClusterCmd.Run(cmd, []string{clusterName})
		}

		fmt.Println("")
		printSuccess("AppPack initialization complete")
		fmt.Print("You can now start installing apps onto your cluster.\n")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&region, "region", "", "AWS region to create resources in")
	initCmd.Flags().String("cluster-name", "apppack", "name of initial cluster")
}
