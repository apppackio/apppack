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
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/apppackio/apppack/cmd"
	"github.com/apppackio/apppack/version"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

var SentryDSN string

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Fprintln(os.Stderr, "\n\nKeyboard interrupt detected, exiting...")
		showCursor()
		os.Exit(130)
	}()

	if SentryDSN != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:           SentryDSN,
			SampleRate:    0,
			EnableTracing: false,
			Release:       version.Version,
			Environment:   version.Environment,
			ServerName:    "apppack",
		})
		if err != nil {
			log.Fatalf("sentry.Init: %s", err)
		}
	}

	// Registered unconditionally so behaviour is identical whether or not
	// SentryDSN was set at build time -- a panic must always exit non-zero.
	defer handlePanic()

	cmd.Execute()
}

// handlePanic recovers from a panic anywhere in the CLI, reports it to
// Sentry (if configured), prints a message to stderr, and exits with a
// non-zero status so callers (CI, shell scripts, etc.) can detect the
// failure. If this is not deferred as the outermost recover, the panic
// resumes and the process crashes normally.
func handlePanic() {
	err := recover()
	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, aurora.Faint(fmt.Sprintf("%v", err)))
	fmt.Fprintln(os.Stderr, aurora.Red("✖"), "Something went wrong. Please retry.")
	fmt.Fprintln(os.Stderr, "  Contact support if the issue persists.")

	// Safe even when sentry.Init was never called: CurrentHub() returns a
	// hub with a no-op client in that case.
	sentry.CurrentHub().Recover(err)
	sentry.Flush(time.Second * 3)

	showCursor()
	os.Exit(1)
}

// showCursor sends the terminal a command to show the cursor on
func showCursor() {
	if runtime.GOOS != "windows" {
		fmt.Print("\033[?25h")
	}
}
