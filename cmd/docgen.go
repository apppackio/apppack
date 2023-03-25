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
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var directory string

func filePrepender(filename string) string {
	name := filepath.Base(filename)
	base := strings.TrimSuffix(name, path.Ext(name))
	commandParts := strings.Split(base, "_")
	if len(commandParts) > 1 {
		commandParts = commandParts[1:]
	} else {
		commandParts = append(commandParts, "(base command)")
	}
	return fmt.Sprintf(`---
title: %s
---

`, strings.Join(commandParts, " "))
}

// docgenCmd represents the docgen command
var docgenCmd = &cobra.Command{
	Use:    "docgen",
	Short:  "generate command documentation as markdown",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		checkErr(os.MkdirAll(directory, os.FileMode(0o750)))
		identity := func(s string) string { return s }
		checkErr(doc.GenMarkdownTreeCustom(rootCmd, directory, filePrepender, identity))
	},
}

func init() {
	rootCmd.AddCommand(docgenCmd)
	docgenCmd.Flags().StringVarP(&directory, "directory", "d", "./docs", "output directory")
}
