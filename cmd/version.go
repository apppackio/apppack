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
	"encoding/json"
	"fmt"

	"github.com/apppackio/apppack/version"
	"github.com/spf13/cobra"
)

// versionInfo is a JSON-serializable representation of version information.
type versionInfo struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	BuildDate   string `json:"build_date"`
	Environment string `json:"environment"`
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:                   "version",
	Short:                 "show the version of the apppack command",
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		if AsJSON {
			info := versionInfo{
				Version:     version.Version,
				Commit:      version.Commit,
				BuildDate:   version.BuildDate,
				Environment: version.Environment,
			}

			out, err := json.MarshalIndent(info, "", "  ")
			checkErr(err)
			fmt.Println(string(out))

			return
		}

		if version.Environment != "production" {
			fmt.Println(version.Environment)
		} else {
			fmt.Println(version.Version)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
