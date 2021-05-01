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
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/spf13/cobra"
)

func ec2InstanceTypes(sess *session.Session) ([]*ec2.InstanceTypeInfo, error) {
	ec2Svc := ec2.New(sess)
	input := ec2.DescribeInstanceTypesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("current-generation"),
				Values: []*string{aws.String("true")},
			},
			{
				Name:   aws.String("processor-info.supported-architecture"),
				Values: []*string{aws.String("x86_64")},
			},
		},
	}
	instanceTypes := []*ec2.InstanceTypeInfo{}
	err := ec2Svc.DescribeInstanceTypesPages(&input, func(page *ec2.DescribeInstanceTypesOutput, lastPage bool) bool {
		for _, instanceType := range page.InstanceTypes {
			if instanceType == nil {
				continue
			}
			instanceTypes = append(instanceTypes, instanceType)
		}
		return !lastPage
	})
	if err != nil {
		return nil, err
	}
	return instanceTypes, nil
}

func instanceTypeNames(sess *session.Session) ([]string, error) {
	instanceTypes, err := ec2InstanceTypes(sess)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, i := range instanceTypes {
		names = append(names, *i.InstanceType)
	}
	sort.Slice(names, func(i int, j int) bool {
		return instanceNameWeight(names[i]) < instanceNameWeight(names[j])
	})
	return names, nil
}

// createClusterCmd represents the create command
var createClusterCmd = &cobra.Command{
	Use:                   "cluster [<name>]",
	Short:                 "setup resources for an AppPack Cluster",
	Long:                  "*Requires AWS credentials.*\nCreates an AppPack Cluster. If a `<name>` is not provided, the default name, `apppack` will be used.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var clusterName string
		if len(args) == 0 {
			clusterName = "apppack"
		} else {
			clusterName = args[0]
		}
		questions := []*survey.Question{}
		answers := make(map[string]interface{})
		addQuestionFromFlag(cmd.Flags().Lookup("domain"), &questions, nil)
		startSpinner()
		sess, err := awsSession()
		checkErr(err)
		_, err = stackFromDDBItem(sess, fmt.Sprintf("CLUSTER#%s", clusterName))
		if err == nil {
			checkErr(fmt.Errorf("cluster %s already exists", clusterName))
		}
		if isTruthy(aws.String(cmd.Flags().Lookup("ec2").Value.String())) {
			Spinner.Suffix = " looking up EC2 instance types"
			instanceClasses, err := instanceTypeNames(sess)
			Spinner.Suffix = ""
			checkErr(err)
			addQuestionFromFlag(cmd.Flags().Lookup("instance-class"), &questions, &survey.Question{
				Name:   "instance-class",
				Prompt: &survey.Select{Message: "select an instance class", Options: instanceClasses, FilterMessage: "", Default: "t3.medium"},
			})
			checkErr(err)
		}
		Spinner.Stop()
		err = survey.Ask(questions, &answers)
		checkErr(err)
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("apppack:cluster"), Value: &clusterName},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}
		domain := getArgValue(cmd, &answers, "domain", true)
		zone, err := hostedZoneForDomain(sess, *domain)
		zoneID := strings.Split(*zone.Id, "/")[2]
		checkErr(err)
		input := cloudformation.CreateStackInput{
			StackName:   aws.String(fmt.Sprintf("apppack-cluster-%s", clusterName)),
			TemplateURL: aws.String(getReleaseUrl(clusterFormationURL)),
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
					ParameterValue: getArgValue(cmd, &answers, "instance-class", false),
				},
				{
					ParameterKey:   aws.String("Domain"),
					ParameterValue: getArgValue(cmd, &answers, "domain", true),
				},
				{
					ParameterKey:   aws.String("HostedZone"),
					ParameterValue: &zoneID,
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
	// All flags need to be added to `initCmd` as well so it can call this cmd
	createClusterCmd.Flags().StringP("domain", "d", "", "parent domain for apps in the cluster")
	initCmd.Flags().StringP("domain", "d", "", "parent domain for apps in the cluster")
	createClusterCmd.Flags().BoolP("ec2", "e", false, "setup cluster with EC2 instances")
	initCmd.Flags().BoolP("ec2", "e", false, "setup cluster with EC2 instances")
	createClusterCmd.Flags().StringP("instance-class", "i", "", "autoscaling instance class -- see https://aws.amazon.com/ec2/pricing/on-demand/")
	initCmd.Flags().StringP("instance-class", "i", "", "autoscaling instance class -- see https://aws.amazon.com/ec2/pricing/on-demand/")
}
