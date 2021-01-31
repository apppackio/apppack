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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/spf13/cobra"
)


// createClusterCmd represents the create command
var createClusterCmd = &cobra.Command{
	Use:                   "cluster [<name>]",
	Short:                 "setup resources for an AppPack Cluster",
	Long:                  "*Requires AWS credentials.*\nCreates an AppPack Cluster. If a `<name>` is not provided, the default name, `apppack` will be used.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		answers, err := askForMissingArgs(cmd, nil)
		var clusterName string
		if len(args) == 0 {
			clusterName = "apppack"
		} else {
			clusterName = args[0]
		}
		checkErr(err)
		sess, err := awsSession()
		checkErr(err)
		_, err = stackFromDDBItem(sess, fmt.Sprintf("CLUSTER#%s", clusterName))
		if err == nil {
			checkErr(fmt.Errorf("cluster %s already exists", clusterName))
		}
		if createChangeSet {
			fmt.Println("Creating Cloudformation Change Set for cluster resources...")
		} else {
			fmt.Println("Creating cluster resources...")
		}
		startSpinner()
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("apppack:cluster"), Value: &clusterName},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}
		domain := getArgValue(cmd, answers, "domain", true)
		zone, err := hostedZoneForDomain(sess, *domain)
		zoneId := strings.Split(*zone.Id, "/")[2]
		checkErr(err)
		input := cloudformation.CreateStackInput{
			StackName:   aws.String(fmt.Sprintf("apppack-cluster-%s", clusterName)),
			TemplateURL: aws.String(clusterFormationURL),
			Parameters: []*cloudformation.Parameter{
				{
					ParameterKey:   aws.String("Name"),
					ParameterValue: &clusterName,
				},
				{
					ParameterKey: aws.String("AvailabilityZones"),
					ParameterValue: aws.String(strings.Join(
						[]string{fmt.Sprintf("%sa", *sess.Config.Region), fmt.Sprintf("%sb", *sess.Config.Region), fmt.Sprintf("%sc", *sess.Config.Region)},
						",",
					)),
				},
				{
					ParameterKey:   aws.String("InstanceType"),
					ParameterValue: getArgValue(cmd, answers, "instance-class", false),
				},
				{
					ParameterKey:   aws.String("Domain"),
					ParameterValue: getArgValue(cmd, answers, "domain", true),
				},
				{
					ParameterKey:   aws.String("HostedZone"),
					ParameterValue: &zoneId,
				},
			},
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags:         cfnTags,
		}
		err = createStackOrChangeSet(sess, &input, createChangeSet, fmt.Sprintf("%s cluster", clusterName))
		checkErr(err)
	},
}

func init() {
	createCmd.AddCommand(createClusterCmd)
	createClusterCmd.Flags().StringP("domain", "d", "", "parent domain for apps in the cluster")
	createClusterCmd.Flags().StringP("instance-class", "i", "t3.medium", "autoscaling instance class -- see https://aws.amazon.com/ec2/pricing/on-demand/")
}
