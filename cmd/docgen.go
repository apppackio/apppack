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

func newDocgenCmd() *cobra.Command {
	var directory string

	cmd := &cobra.Command{
		Use:    "docgen",
		Short:  "generate command documentation as markdown",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Documents the tree this command is attached to, so a test can
			// generate from its own tree rather than the package-level one.
			return generateDocs(cmd.Root(), directory)
		},
	}

	cmd.Flags().StringVarP(&directory, "directory", "d", "./docs", "output directory")

	return cmd
}

func generateDocs(root *cobra.Command, directory string) error {
	if err := os.MkdirAll(directory, os.FileMode(0o750)); err != nil {
		return err
	}

	identity := func(s string) string { return s }

	return doc.GenMarkdownTreeCustom(root, directory, filePrepender, identity)
}

func init() {
	registerCommand(newDocgenCmd)
}
