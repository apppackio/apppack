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
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/apppackio/apppack/selfupdate"
	"github.com/apppackio/apppack/ui"
	"github.com/apppackio/apppack/version"
	"github.com/cli/safeexec"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

// versionInfo is a JSON-serializable representation of version information.
type versionInfo struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	BuildDate   string `json:"build_date"`
	Environment string `json:"environment"`
}

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "version",
		Short:                 "show the version of the apppack command",
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Read the flag off the command rather than the package-level
			// AsJSON, so two command trees in the same test binary do not
			// share it.
			asJSON, err := cmd.Flags().GetBool("json")
			if err != nil {
				return err
			}

			return runVersion(cmd.OutOrStdout(), asJSON)
		},
	}

	cmd.AddCommand(newVersionCheckCmd())
	cmd.AddCommand(newVersionUpdateCmd())

	return cmd
}

func runVersion(out io.Writer, asJSON bool) error {
	if asJSON {
		return fprintJSON(out, versionInfo{
			Version:     version.Version,
			Commit:      version.Commit,
			BuildDate:   version.BuildDate,
			Environment: version.Environment,
		})
	}

	// A non-production build reports the environment it was built for --
	// "development" for a local `go build`, where Version is not stamped.
	if version.Environment != "production" {
		_, err := fmt.Fprintln(out, version.Environment)

		return err
	}

	_, err := fmt.Fprintln(out, version.Version)

	return err
}

func newVersionCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:                   "check",
		Short:                 "check if a newer version is available",
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ui.StartSpinner()
			ui.Spinner.Suffix = " checking for updates..."

			release, err := version.GetLatestReleaseInfo(cmd.Context(), http.DefaultClient, repo)

			ui.Spinner.Stop()

			if err != nil {
				return err
			}

			return reportUpdateAvailable(cmd.OutOrStdout(), release.Version)
		},
	}
}

func reportUpdateAvailable(out io.Writer, latest string) error {
	if !version.VersionGreaterThan(latest, version.Version) {
		_, err := fmt.Fprintln(out, aurora.Green("✔ "+fmt.Sprintf("Already up to date (version %s)", strings.TrimPrefix(version.Version, "v"))))

		return err
	}

	if _, err := fmt.Fprintf(out, "%s %s → %s\n",
		aurora.Yellow("Update available:"),
		aurora.Cyan(strings.TrimPrefix(version.Version, "v")),
		aurora.Cyan(strings.TrimPrefix(latest, "v")),
	); err != nil {
		return err
	}

	_, err := fmt.Fprintf(out, "Run %s to update\n", aurora.White("apppack version update"))

	return err
}

type versionUpdateOptions struct {
	force bool
}

func newVersionUpdateCmd() *cobra.Command {
	o := &versionUpdateOptions{}

	cmd := &cobra.Command{
		Use:                   "update",
		Short:                 "update apppack to the latest version",
		Long:                  "Downloads and installs the latest version of apppack from GitHub releases.",
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return o.run(cmd.Context(), cmd.OutOrStdout())
		},
	}

	cmd.Flags().BoolVarP(&o.force, "force", "f", false, "force update even if already on latest version")

	return cmd
}

func (o *versionUpdateOptions) run(ctx context.Context, out io.Writer) error {
	appPath, err := safeexec.LookPath(os.Args[0])
	if err != nil {
		return err
	}

	// Homebrew owns the binary it installed; replacing it underneath brew
	// leaves the formula and the file disagreeing.
	if IsUnderHomebrew(appPath) {
		_, _ = fmt.Fprintln(out, aurora.Yellow("⚠  AppPack was installed via Homebrew"))
		_, err = fmt.Fprintf(out, "To update, run: %s\n", aurora.White("brew upgrade apppack"))

		return err
	}

	ui.StartSpinner()
	ui.Spinner.Suffix = " checking for updates..."

	release, err := version.GetLatestReleaseInfo(ctx, http.DefaultClient, repo)
	if err != nil {
		ui.Spinner.Stop()

		return err
	}

	if !o.force && !version.VersionGreaterThan(release.Version, version.Version) {
		ui.Spinner.Stop()
		_, err = fmt.Fprintln(out, aurora.Green("✔ "+fmt.Sprintf("Already up to date (version %s)", strings.TrimPrefix(version.Version, "v"))))

		return err
	}

	ui.Spinner.Suffix = fmt.Sprintf(" downloading %s...", release.Version)

	err = selfupdate.Update(ctx, http.DefaultClient, release, appPath)

	ui.Spinner.Stop()

	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, aurora.Green("✔ Updated to version "+strings.TrimPrefix(release.Version, "v")))

	return err
}

func init() {
	registerCommand(newVersionCmd)
}
