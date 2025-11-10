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
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/spf13/cobra"
)

// This code is taken almost verbatim from aws-vault
// https://github.com/99designs/aws-vault/blob/0c10c45693116608fb3765be304f554189f0df01/cli/exec.go

// The MIT License (MIT)

// Copyright (c) 2015 99designs

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

func execEnvironment(command string, args []string, cfg aws.Config) error {
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		// return fmt.Errorf("Failed to get credentials for %s: %w", input.ProfileName, err)
		return err
	}

	env := environ(os.Environ())

	// log.Println("Setting subprocess env: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY")
	env.Set("AWS_ACCESS_KEY_ID", creds.AccessKeyID)
	env.Set("AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey)

	if creds.SessionToken != "" {
		// log.Println("Setting subprocess env: AWS_SESSION_TOKEN, AWS_SECURITY_TOKEN")
		env.Set("AWS_SESSION_TOKEN", creds.SessionToken)
		env.Set("AWS_SECURITY_TOKEN", creds.SessionToken)
	}

	if !creds.Expires.IsZero() {
		// log.Println("Setting subprocess env: AWS_SESSION_EXPIRATION")
		env.Set("AWS_SESSION_EXPIRATION", creds.Expires.UTC().Format(time.RFC3339))
	}

	if !supportsExecSyscall() {
		return execCmd(command, args, env)
	}

	return execSyscall(command, args, env)
}

// environ is a slice of strings representing the environment, in the form "key=value".
type environ []string

// Unset an environment variable by key
func (e *environ) Unset(key string) {
	for i := range *e {
		if strings.HasPrefix((*e)[i], key+"=") {
			(*e)[i] = (*e)[len(*e)-1]
			*e = (*e)[:len(*e)-1]

			break
		}
	}
}

// Set adds an environment variable, replacing any existing ones of the same key
func (e *environ) Set(key, val string) {
	e.Unset(key)
	*e = append(*e, key+"="+val)
}

func execCmd(command string, args, env []string) error {
	cmd := osexec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		for {
			sig := <-sigChan
			_ = cmd.Process.Signal(sig)
		}
	}()

	if err := cmd.Wait(); err != nil {
		_ = cmd.Process.Signal(os.Kill)

		return fmt.Errorf("failed to wait for command termination: %w", err)
	}

	waitStatus := cmd.ProcessState.Sys().(syscall.WaitStatus)
	os.Exit(waitStatus.ExitStatus())

	return nil
}

func supportsExecSyscall() bool {
	return runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd"
}

func execSyscall(command string, args, env []string) error {
	argv0, err := osexec.LookPath(command)
	if err != nil {
		return fmt.Errorf("couldn't find the executable '%s': %w", command, err)
	}

	// log.Printf("Found executable %s", argv0)

	argv := make([]string, 0, 1+len(args))
	argv = append(argv, command)
	argv = append(argv, args...)

	return syscall.Exec(argv0, argv, env)
}

// awsExecCmd represents the aws-exec command
var awsExecCmd = &cobra.Command{
	Use:                   "aws-exec -- <command>...",
	Short:                 "run a local command with AWS credentials",
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, args []string) {
		a, err := app.Init(AppName, UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		if len(args) < 1 {
			checkErr(errors.New("provide an executable to run as an argument"))
		}
		err = execEnvironment(args[0], args[1:], a.Session)
		checkErr(err)
	},
}

func init() {
	rootCmd.AddCommand(awsExecCmd)
	awsExecCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	awsExecCmd.MarkPersistentFlagRequired("app-name")
}
