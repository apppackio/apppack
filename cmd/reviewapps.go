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
package cmd

import (
	"fmt"
	"math/rand"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"

	"github.com/spf13/cobra"
)

func reviewAppStackName(pipeline string, prNumber string) string {
	return fmt.Sprintf("apppack-reviewapp-%s%s", pipeline, prNumber)
}

func getPipelineStack(a *app.App) (*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(a.Session)
	stacks, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: aws.String(pipelineStackName(a.Name)),
	})
	if err != nil {
		return nil, err
	}
	return stacks.Stacks[0], nil
}

// reviewappsCmd represents the reviewapps command
var reviewappsCmd = &cobra.Command{
	Use:                   "reviewapps",
	Short:                 "manage review apps",
	Long:                  ``,
	DisableFlagsInUseLine: true,
}

// reviewappsStatusCmd represents the status command
var reviewappsStatusCmd = &cobra.Command{
	Use:                   "status <pipeline>",
	Short:                 "",
	Long:                  ``,
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(args[0])
		checkErr(err)
		reviewApps, err := a.GetReviewApps()
		checkErr(err)
		Spinner.Stop()
		for _, r := range reviewApps {
			fmt.Printf("%s (%s): %s\n", r.PullRequest, r.Branch, r.Status)
		}
	},
}

// reviewappsCreateCmd represents the create command
var reviewappsCreateCmd = &cobra.Command{
	Use:                   "create <pipeline:pr-number>",
	Short:                 "",
	Long:                  ``,
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(args[0])
		checkErr(err)
		if !a.IsReviewApp() {  // TODO: validate
			checkErr(fmt.Errorf("no pull request number set"))
		}
		stack, err := getPipelineStack(a)
		checkErr(err)
		cfnRoleArn, err := getStackOutput(stack, "ReviewAppCfnRoleArn")
		checkErr(err)
		databaseLambda, err := getStackOutput(stack, "DatabaseManagerLambdaArn")
		checkErr(err)
		var databaseAddon string
		if *databaseLambda == "~" {
			databaseAddon = "disabled"
		} else {
			databaseAddon = "enabled"
		}
		redisLambda, err := getStackOutput(stack, "RedisManagerLambdaArn")
		checkErr(err)
		var redisAddon string
		if *redisLambda == "~" {
			redisAddon = "disabled"
		} else {
			redisAddon = "enabled"
		}
		cfnSvc := cloudformation.New(a.Session)
		_, err = createStackAndWait(cfnSvc, &cloudformation.CreateStackInput{
			StackName:   aws.String(reviewAppStackName(a.Name, *a.ReviewApp)),
			TemplateURL: aws.String(getReleaseUrl("https://s3.amazonaws.com/apppack-cloudformations/latest/review-app.json")),
			RoleARN:     cfnRoleArn,
			Parameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("DatabaseAddon"), ParameterValue: &databaseAddon},
				{ParameterKey: aws.String("RedisAddon"), ParameterValue: &redisAddon},
				{
					ParameterKey:   aws.String("Name"),
					ParameterValue: a.ReviewApp,
				},
				{
					ParameterKey:   aws.String("PipelineStackName"),
					ParameterValue: aws.String(pipelineStackName(a.Name)),
				},
				{
					ParameterKey:   aws.String("LoadBalancerRulePriority"),
					ParameterValue: aws.String(fmt.Sprintf("%d", rand.Intn(50000-1)+1)),
				},
			},
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags: []*cloudformation.Tag{
				{Key: aws.String("apppack:appName"), Value: &a.Name},
				{Key: aws.String("apppack:reviewApp"), Value: aws.String(fmt.Sprintf("pr%s", *a.ReviewApp))},
				//{Key: aws.String("apppack:cluster"), Value: aws.String("...")},
				{Key: aws.String("apppack"), Value: aws.String("true")},
			},
		}, true)
		checkErr(err)
		// codebuildSvc := codebuild.New(p.App.Session)
		// err = p.App.LoadSettings()
		// checkErr(err)
		// ssmSvc := ssm.New(p.App.Session)
		// parameterName := fmt.Sprintf("/apppack/pipelines/%s/review-apps/pr/%s", p.App.Name, pr)
		// parameterOutput, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
		// 	Name: &parameterName,
		// })
		// checkErr(err)
		// r := pipeline.ReviewApp{}
		// err = json.Unmarshal([]byte(*parameterOutput.Parameter.Value), &r)
		// checkErr(err)
		// codebuildSvc.StartBuild(&codebuild.StartBuildInput{
		// 	ProjectName:   &p.App.Settings.CodebuildProject.Name,
		// 	SourceVersion: &r.PullRequest,
		// 	EnvironmentVariablesOverride: []*codebuild.EnvironmentVariable{
		// 		{
		// 			Name:  aws.String("BRANCH"),
		// 			Value: &r.Branch,
		// 			Type:  aws.String("PLAINTEXT"),
		// 		}, {
		// 			Name:  aws.String("REVIEW_APP_STATUS"),
		// 			Value: aws.String("creating"),
		// 			Type:  aws.String("PLAINTEXT"),
		// 		},
		// 	},
		// })
		// Spinner.Stop()
		printSuccess(fmt.Sprintf("deploying review app for PR #%s", *a.ReviewApp))
	},
}

// reviewappsDestroyCmd represents the destroy command
var reviewappsDestroyCmd = &cobra.Command{
	Use:                   "destroy <pipeline:pr-number>",
	Short:                 "",
	Long:                  ``,
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(args[0])
		checkErr(err)
		if !a.IsReviewApp() {  // TODO: validate
			checkErr(fmt.Errorf("no pull request number set"))
		}
		stackName := reviewAppStackName(a.Name, *a.ReviewApp)
		cfnSvc := cloudformation.New(a.Session)
		friendlyName := fmt.Sprintf("%s review app for PR#%s", a.Name, *a.ReviewApp)
		stack, err := confirmDeleteStack(cfnSvc, stackName, friendlyName)
		checkErr(err)
		err = deleteStack(cfnSvc, *stack.StackId, friendlyName, true)
		checkErr(err)
	},
}

func init() {
	rootCmd.AddCommand(reviewappsCmd)
	reviewappsCmd.AddCommand(reviewappsStatusCmd)
	reviewappsCmd.AddCommand(reviewappsCreateCmd)
	reviewappsCreateCmd.Flags().StringVar(&release, "release", "", "Specify a specific pre-release stack")
	upgradeCmd.PersistentFlags().MarkHidden("release")
	reviewappsCmd.AddCommand(reviewappsDestroyCmd)
	//reviewappsCmd.AddCommand(configCmd)
}
