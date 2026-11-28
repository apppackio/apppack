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
	"io"

	"github.com/apppackio/apppack/app"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

type openOptions struct {
	appName           string
	useAWSCredentials bool
}

func newOpenCmd() *cobra.Command {
	o := &openOptions{}

	cmd := &cobra.Command{
		Use:   "open",
		Short: "open the app in a browser",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return o.run(cmd.OutOrStdout())
		},
	}

	cmd.PersistentFlags().StringVarP(&o.appName, "app-name", "a", "", "app name (required)")
	_ = cmd.MarkPersistentFlagRequired("app-name")
	cmd.PersistentFlags().BoolVar(&o.useAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")

	return cmd
}

func (o *openOptions) run(out io.Writer) error {
	a, err := app.Init(o.appName, o.useAWSCredentials, MaxSessionDurationSeconds)
	if err != nil {
		return err
	}

	u, err := a.URL(nil)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "opening %s\n", aurora.Bold(*u))

	if err := browser.OpenURL(*u); err != nil {
		_, _ = fmt.Fprintln(out, "Open this URL in your browser to view it:")
		_, _ = fmt.Fprintln(out, *u)
	}

	return nil
}

func init() {
	registerCommand(newOpenCmd)
}
