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

	"github.com/AlecAivazis/survey/v2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/spf13/cobra"
)

// createRegionCmd represents the create command
var createRegionCmd = &cobra.Command{
	Use:                   "region",
	Short:                 "setup AppPack resources for an AWS region",
	Long:                  "*Requires AWS credentials.*",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		sess, err := awsSession()
		checkErr(err)
		questions := []*survey.Question{}
		answers := make(map[string]interface{})
		addQuestionFromFlag(cmd.Flags().Lookup("dockerhub-username"), &questions, nil)
		addQuestionFromFlag(cmd.Flags().Lookup("dockerhub-access-token"), &questions, nil)
		err = survey.Ask(questions, &answers)
		checkErr(err)
		ssmSvc := ssm.New(sess)
		if createChangeSet {
			fmt.Println("Creating Cloudformation Change Set for region-level resources...")
		} else {
			fmt.Println("Creating region-level resources...")
		}
		startSpinner()
		region := sess.Config.Region
		tags := []*ssm.Tag{
			{Key: aws.String("apppack:region"), Value: region},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}
		_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
			Name:  aws.String("/apppack/account/dockerhub-access-token"),
			Value: getArgValue(cmd, &answers, "dockerhub-access-token", true),
			Type:  aws.String("SecureString"),
			Tags:  tags,
		})
		checkErr(err)
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("apppack:region"), Value: region},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}

		input := cloudformation.CreateStackInput{
			StackName:   aws.String(fmt.Sprintf("apppack-region-%s", *region)),
			TemplateURL: aws.String(regionFormationURL),
			Parameters: []*cloudformation.Parameter{
				{
					ParameterKey:   aws.String("DockerhubUsername"),
					ParameterValue: getArgValue(cmd, &answers, "dockerhub-username", true),
				},
			},
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags:         cfnTags,
		}
		err = createStackOrChangeSet(sess, &input, createChangeSet, fmt.Sprintf("%s region", *region))
		checkErr(err)
	},
}

func init() {
	createCmd.AddCommand(createRegionCmd)
	// All flags need to be added to `initCmd` as well so it can call this cmd
	createRegionCmd.Flags().StringP("dockerhub-username", "u", "", "Docker Hub username")
	initCmd.Flags().StringP("dockerhub-username", "u", "", "Docker Hub username")
	createRegionCmd.Flags().StringP("dockerhub-access-token", "t", "", "Docker Hub Access Token (https://hub.docker.com/settings/security)")
	initCmd.Flags().StringP("dockerhub-access-token", "t", "", "Docker Hub Access Token (https://hub.docker.com/settings/security)")

}
