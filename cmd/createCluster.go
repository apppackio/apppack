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
	"net"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ddb"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
)

// splitSubnet takes a CIDR string and splits it up into two groups of 3 comma-separated subnets
func splitSubnet(cidrStr string) (*string, *string, error) {
	maxSubnetMask := 24
	minSubnetMask := 16
	_, clusterCIDR, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return nil, nil, err
	}
	mask, _ := clusterCIDR.Mask.Size()
	if mask < minSubnetMask || mask > maxSubnetMask {
		return nil, nil, fmt.Errorf("valid subnet mask range is %d-%d", minSubnetMask, maxSubnetMask)
	}
	subnetMask := net.CIDRMask(mask+4, 32)
	subnets := []*net.IPNet{}
	subnets = append(subnets, &net.IPNet{IP: clusterCIDR.IP, Mask: subnetMask})
	prefix, _ := subnetMask.Size()
	for i := 0; i < 8; i++ {
		nextCIDR, _ := cidr.NextSubnet(subnets[i], prefix)
		subnets = append(subnets, nextCIDR)
	}
	publicSubnets := []string{}
	for i := 0; i < 3; i++ {
		publicSubnets = append(publicSubnets, subnets[i].String())
	}
	privateSubnets := []string{}
	for i := 6; i < 9; i++ {
		privateSubnets = append(privateSubnets, subnets[i].String())
	}
	return aws.String(strings.Join(publicSubnets, ",")), aws.String(strings.Join(privateSubnets, ",")), nil
}

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
	Long:                  "*Requires admin permissions.*\nCreates an AppPack Cluster. If a `<name>` is not provided, the default name, `apppack` will be used.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var clusterName string
		if len(args) == 0 {
			clusterName = "apppack"
		} else {
			clusterName = args[0]
		}
		startSpinner()
		sess, err := adminSession(MaxSessionDurationSeconds)
		checkErr(err)
		regionExists, err := bridge.StackExists(sess, fmt.Sprintf("apppack-region-%s", *sess.Config.Region))
		checkErr(err)
		if !*regionExists {
			Spinner.Stop()
			fmt.Println(aurora.Blue(fmt.Sprintf("ℹ %s region is not initialized", *sess.Config.Region)))
			fmt.Printf("If this is your first cluster or you want to setup up a new region, type '%s' to continue.\n", aurora.White("yes"))
			fmt.Print(aurora.White(fmt.Sprintf("Create cluster in %s region? ", *sess.Config.Region)).String())
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "yes" {
				checkErr(fmt.Errorf("aborting due to user input"))
			}
			fmt.Printf("running %s...\n", aurora.White("apppack create region"))
			createRegionCmd.Run(cmd, []string{})
			fmt.Println("")
		}
		questions := []*survey.Question{}
		answers := make(map[string]interface{})
		addQuestionFromFlag(cmd.Flags().Lookup("domain"), &questions, nil)

		_, err = ddb.StackFromItem(sess, fmt.Sprintf("CLUSTER#%s", clusterName))
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
		zone, err := bridge.HostedZoneForDomain(sess, *domain)
		checkErr(err)
		checkErr(checkHostedZone(sess, zone))
		zoneID := strings.Split(*zone.Id, "/")[2]
		checkErr(err)
		vpcCIDR := getArgValue(cmd, &answers, "cidr", true)
		publicSubnets, privateSubnets, err := splitSubnet(*vpcCIDR)
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
					ParameterKey:   aws.String("Cidr"),
					ParameterValue: vpcCIDR,
				},
				{
					ParameterKey:   aws.String("PublicSubnetCidrs"),
					ParameterValue: publicSubnets,
				},
				{
					ParameterKey:   aws.String("PrivateSubnetCidrs"),
					ParameterValue: privateSubnets,
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
	createClusterCmd.Flags().String("domain", "", "cluster domain name")
	createClusterCmd.Flags().Bool("ec2", false, "setup cluster with EC2 instances")
	createClusterCmd.Flags().String("instance-class", "", "autoscaling instance class -- see https://aws.amazon.com/ec2/pricing/on-demand/")
	createClusterCmd.Flags().String("cidr", "10.100.0.0/16", "network CIDR for VPC")
	// from createRegion
	createClusterCmd.Flags().String("dockerhub-username", "", "Docker Hub username")
	createClusterCmd.Flags().String("dockerhub-access-token", "", "Docker Hub Access Token (https://hub.docker.com/settings/security)")
	createClusterCmd.Flags().MarkHidden("dockerhub-username")
	createClusterCmd.Flags().MarkHidden("dockerhub-access-token")
}
