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

	"github.com/apppackio/apppack/pipeline"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"

	"github.com/spf13/cobra"
)

var PipelineName string

// reviewappsCmd represents the reviewapps command
var reviewappsCmd = &cobra.Command{
	Use:                   "reviewapps",
	Short:                 "manage review apps",
	Long:                  ``,
	DisableFlagsInUseLine: true,
}

// reviewappsStatusCmd represents the status command
var reviewappsStatusCmd = &cobra.Command{
	Use:                   "status",
	Short:                 "",
	Long:                  ``,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		p, err := pipeline.Init(PipelineName)
		checkErr(err)
		reviewApps, err := p.GetReviewApps()
		checkErr(err)
		Spinner.Stop()
		for _, r := range reviewApps {
			fmt.Printf("%s (%s): %s\n", r.PullRequest, r.Branch, r.Status)
		}
	},
}

// reviewappsCreateCmd represents the create command
var reviewappsCreateCmd = &cobra.Command{
	Use:                   "create",
	Short:                 "",
	Long:                  ``,
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pr := args[0]
		startSpinner()
		p, err := pipeline.Init(PipelineName)
		checkErr(err)
		cfnSvc := cloudformation.New(p.App.Session)
		prNumber := args[0] // TODO validate
		stacks, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: aws.String(pipelineStackName(PipelineName)),
		})
		checkErr(err)
		var cfnRoleArn *string
		for _, o := range stacks.Stacks[0].Outputs {
			if *o.OutputKey == "ReviewAppCfnRoleArn" {
				cfnRoleArn = o.OutputValue
				break
			}
		}
		_, err = createStackAndWait(cfnSvc, &cloudformation.CreateStackInput{
			StackName:   aws.String(fmt.Sprintf("apppack-reviewapp-%s%s", PipelineName, prNumber)),
			TemplateURL: aws.String("https://s3.amazonaws.com/apppack-cloudformations/latest/review-app.json"),
			RoleARN:     cfnRoleArn,
			Parameters: []*cloudformation.Parameter{
				{
					ParameterKey:   aws.String("Name"),
					ParameterValue: &prNumber,
				},
				{
					ParameterKey:   aws.String("PipelineStackName"),
					ParameterValue: aws.String(pipelineStackName(PipelineName)),
				},
				{
					ParameterKey:   aws.String("LoadBalancerRulePriority"),
					ParameterValue: aws.String(fmt.Sprintf("%d", rand.Intn(50000-1)+1)),
				},
			},
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags: []*cloudformation.Tag{
				{Key: aws.String("apppack:appName"), Value: &PipelineName},
				{Key: aws.String("apppack:reviewApp"), Value: aws.String(fmt.Sprintf("pr%s", prNumber))},
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
		printSuccess(fmt.Sprintf("deploying review app for PR #%s", pr))
	},
}

// reviewappsDestroyCmd represents the destroy command
var reviewappsDestroyCmd = &cobra.Command{
	Use:                   "destroy",
	Short:                 "",
	Long:                  ``,
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pr := args[0]
		startSpinner()
		p, err := pipeline.Init(PipelineName)
		checkErr(err)
		err = p.SetReviewAppStatus(pr, "pending_destroy")
		checkErr(err)
		Spinner.Stop()
		printSuccess(fmt.Sprintf("creating review app for PR #%s", pr))
	},
}

func init() {
	rootCmd.AddCommand(reviewappsCmd)
	reviewappsCmd.PersistentFlags().StringVarP(&PipelineName, "pipeline", "p", "", "pipeline name (required)")
	reviewappsCmd.MarkPersistentFlagRequired("pipeline")
	reviewappsCmd.AddCommand(reviewappsStatusCmd)
	reviewappsCmd.AddCommand(reviewappsCreateCmd)
	reviewappsCmd.AddCommand(reviewappsDestroyCmd)
	//reviewappsCmd.AddCommand(configCmd)
}
