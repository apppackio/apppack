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

		defer func() {
			if err := recover(); err != nil {
				fmt.Println(aurora.Faint(fmt.Sprintf("%v", err)))
				fmt.Println(aurora.Red("✖"), "Something went wrong. Please retry.")
				fmt.Println("  Contact support if the issue persists.")
				sentry.CurrentHub().Recover(err)
				sentry.Flush(time.Second * 3)
			}
		}()
	}

	cmd.Execute()
}

// showCursor sends the terminal a command to show the cursor on
func showCursor() {
	if runtime.GOOS != "windows" {
		fmt.Print("\033[?25h")
	}
}
