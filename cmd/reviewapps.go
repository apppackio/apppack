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
	"strings"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/logrusorgru/aurora"

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
	Use:                   "reviewapps <pipeline>",
	Short:                 "list deployed review apps",
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(args[0])
		checkErr(err)
		reviewApps, err := a.GetReviewApps()
		checkErr(err)
		Spinner.Stop()
		fmt.Println(aurora.Faint("==="), aurora.Bold(aurora.White(fmt.Sprintf("%s review apps", a.Name))))
		for _, r := range reviewApps {
			if r.Status == "created" {
				prNumber := strings.Split(r.PullRequest, "/")[1]
				fmt.Printf("%s %s\n", aurora.White(fmt.Sprintf("#%s %s", prNumber, r.Title)), aurora.Faint(r.Branch))
				url, err := a.URL(&prNumber)
				checkErr(err)
				indent := len(prNumber) + 1
				fmt.Printf("%s %s\n\n", strings.Repeat(" ", indent), aurora.Underline(aurora.Blue(*url)))
			}
		}
	},
}

// pipelineCfnParameters sets parameters for the review app based on output from the pipeline stack
func pipelineCfnParameters(stack *cloudformation.Stack) ([]*cloudformation.Parameter, error) {
	parameters := []*cloudformation.Parameter{}
	databaseLambda, err := getStackOutput(stack, "DatabaseManagerLambdaArn")
	if err != nil {
		return nil, err
	}
	if *databaseLambda == "~" {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("DatabaseAddon"), ParameterValue: aws.String("disabled"),
		})
	} else {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("DatabaseAddon"), ParameterValue: aws.String("enabled"),
		})
	}
	redisStackName, err := getStackParameter(stack, "RedisStackName")
	if err != nil {
		return nil, err
	}
	if *redisStackName == "" {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("RedisAddon"), ParameterValue: aws.String("disabled"),
		})
	} else {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("RedisAddon"), ParameterValue: aws.String("enabled"),
		})
	}
	privateS3Bucket, err := getStackOutput(stack, "PrivateS3Bucket")
	if err != nil {
		return nil, err
	}
	if *privateS3Bucket == "~" {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("PrivateS3BucketEnabled"), ParameterValue: aws.String("disabled"),
		})
	} else {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("PrivateS3BucketEnabled"), ParameterValue: aws.String("enabled"),
		})
	}
	publicS3Bucket, err := getStackOutput(stack, "PublicS3Bucket")
	if err != nil {
		return nil, err
	}
	if *publicS3Bucket == "~" {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("PublicS3BucketEnabled"), ParameterValue: aws.String("disabled"),
		})
	} else {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("PublicS3BucketEnabled"), ParameterValue: aws.String("enabled"),
		})
	}
	ses, err := getStackOutput(stack, "SesDomain")
	if err != nil {
		return nil, err
	}
	if *ses == "~" {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("SesDomain"), ParameterValue: aws.String("disabled"),
		})
	} else {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("SesDomain"), ParameterValue: aws.String("enabled"),
		})
	}
	sqs, err := getStackOutput(stack, "SQSQueueEnabled")
	if err != nil {
		return nil, err
	}

	parameters = append(parameters, &cloudformation.Parameter{
		ParameterKey: aws.String("SQSQueueEnabled"), ParameterValue: sqs,
	})

	customTaskPolicyArn, err := getStackOutput(stack, "CustomTaskPolicyArn")
	if err != nil {
		return nil, err
	}
	if *customTaskPolicyArn == "~" {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("CustomTaskPolicy"), ParameterValue: aws.String("disabled"),
		})
	} else {
		parameters = append(parameters, &cloudformation.Parameter{
			ParameterKey: aws.String("CustomTaskPolicy"), ParameterValue: aws.String("enabled"),
		})
	}

	return parameters, nil
}

// reviewappsCreateCmd represents the create command
var reviewappsCreateCmd = &cobra.Command{
	Use:                   "create <pipeline:pr-number>",
	Short:                 "create a review app",
	Long:                  `Creates a review app from a pull request on the pipeline repository and triggers the iniital build`,
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(args[0])
		checkErr(err)
		if !a.IsReviewApp() { // TODO: validate
			checkErr(fmt.Errorf("no pull request number set"))
		}
		stack, err := getPipelineStack(a)
		checkErr(err)
		cfnRoleArn, err := getStackOutput(stack, "ReviewAppCfnRoleArn")
		checkErr(err)
		parameters, err := pipelineCfnParameters(stack)
		checkErr(err)
		rand.Seed(time.Now().UnixNano())
		parameters = append(parameters, []*cloudformation.Parameter{
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
				ParameterValue: aws.String(fmt.Sprintf("%d", rand.Intn(50000-200)+200)),
			},
		}...)
		err = createStackOrChangeSet(a.Session, &cloudformation.CreateStackInput{
			StackName:    aws.String(reviewAppStackName(a.Name, *a.ReviewApp)),
			TemplateURL:  aws.String(getReleaseUrl("https://s3.amazonaws.com/apppack-cloudformations/latest/review-app.json")),
			RoleARN:      cfnRoleArn,
			Parameters:   parameters,
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags: []*cloudformation.Tag{
				{Key: aws.String("apppack:appName"), Value: &a.Name},
				{Key: aws.String("apppack:reviewApp"), Value: aws.String(fmt.Sprintf("pr%s", *a.ReviewApp))},
				//{Key: aws.String("apppack:cluster"), Value: aws.String("...")},
				{Key: aws.String("apppack"), Value: aws.String("true")},
			},
		}, false, fmt.Sprintf("review app #%s", *a.ReviewApp))
		checkErr(err)
		Spinner.Stop()
		Spinner.Suffix = " triggering initial build..."
		startSpinner()
		build, err := a.StartBuild(true)
		checkErr(err)
		err = watchBuild(a, build)
		checkErr(err)
		Spinner.Stop()
	},
}

// reviewappsDestroyCmd represents the destroy command
var reviewappsDestroyCmd = &cobra.Command{
	Use:                   "destroy <pipeline:pr-number>",
	Short:                 "destroys the review app",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startSpinner()
		a, err := app.Init(args[0])
		checkErr(err)
		if !a.IsReviewApp() { // TODO: validate
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
	reviewappsCmd.AddCommand(reviewappsCreateCmd)
	reviewappsCreateCmd.Flags().StringVar(&release, "release", "", "Specify a specific pre-release stack")
	upgradeCmd.PersistentFlags().MarkHidden("release")
	reviewappsCmd.AddCommand(reviewappsDestroyCmd)
}
