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
	"strings"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/spf13/cobra"
)

func customDomainStackName(domain string) string {
	return fmt.Sprintf("apppack-customdomain-%s", strings.Replace(strings.TrimSuffix(domain, "."), ".", "-", -1))
}

// appList gets a list of app names from DynamoDB
func appList(sess *session.Session) ([]string, error) {
	ddbSvc := dynamodb.New(sess)
	result, err := ddbSvc.Query(&dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id1": {S: aws.String("CLUSTERS")},
		},
	})
	if err != nil {
		return nil, err
	}
	type clusterItem struct {
		SecondaryID string `json:"secondary_id"`
	}
	clusterItems := []clusterItem{}
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &clusterItems)
	if err != nil {
		return nil, err
	}
	appNames := []string{}
	for _, item := range clusterItems {
		if strings.Contains(item.SecondaryID, "#APP#") {
			appNames = append(appNames, strings.Split(item.SecondaryID, "#")[2])
		}
	}
	return appNames, nil
}

// appForDomain returns the app name associated with a given domain
func appForDomain(sess *session.Session, domain string) (*string, error) {
	appNames, err := appList(sess)
	if err != nil {
		return nil, err
	}
	for _, appName := range appNames {
		a := app.App{
			Name:    appName,
			Session: sess,
		}
		a.LoadSettings()
		for _, appDomain := range a.Settings.Domains {
			if appDomain == domain {
				return &appName, nil
			}
		}
	}

	return nil, fmt.Errorf("no app found for domain %s", domain)
}

func clusterStackForApp(sess *session.Session, appName string) (*string, error) {
	cfnSvc := cloudformation.New(sess)
	stacks, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: aws.String(appStackName(appName)),
	})
	if err != nil {
		return nil, err
	}
	for _, parameter := range stacks.Stacks[0].Parameters {
		if *parameter.ParameterKey == "ClusterStackName" {
			return parameter.ParameterValue, nil
		}
	}
	return nil, fmt.Errorf("app stack %s missing ClusterStackName parameter", appName)
}

// createCustomDomainCmd represents the createCustomDomain command
var createCustomDomainCmd = &cobra.Command{
	Use:   "custom-domain <domain-name>...",
	Args:  cobra.MinimumNArgs(1),
	Short: "setup TLS certificate and point one or more domains to an AppPack Cluster",
	Long: `*Requires AWS credentials.*
	
The domain(s) provided must all part of the same parent domain and a Route53 Hosted Zone must already be setup.`,
	Example:               "apppack create custom-domain example.com www.example.com",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		primaryDomain := args[0]
		startSpinner()
		sess, err := awsSession()
		checkErr(err)
		appName, err := appForDomain(sess, primaryDomain)
		checkErr(err)
		clusterStackName, err := clusterStackForApp(sess, *appName)
		checkErr(err)
		zone, err := hostedZoneForDomain(sess, primaryDomain)
		checkErr(err)
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("apppack:customDomain"), Value: &primaryDomain},
			{Key: aws.String("apppack:appName"), Value: appName},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}
		zoneID := strings.Split(*zone.Id, "/")[2]
		parameters := []*cloudformation.Parameter{
			{
				ParameterKey:   aws.String("PrimaryDomain"),
				ParameterValue: &primaryDomain,
			},
			{
				ParameterKey:   aws.String("HostedZone"),
				ParameterValue: &zoneID,
			},
			{
				ParameterKey:   aws.String("ClusterStackName"),
				ParameterValue: clusterStackName,
			},
		}
		if len(args) > 1 {
			for i, domain := range args[1:] {
				if !isHostedZoneForDomain(domain, zone) {
					checkErr(fmt.Errorf("%s can not be placed in the hosted zone %s (%s)", domain, *zone.Name, zoneID))
				}
				parameters = append(parameters, &cloudformation.Parameter{
					ParameterKey:   aws.String(fmt.Sprintf("AltDomain%d", i+1)),
					ParameterValue: &domain,
				})
			}
		}

		input := cloudformation.CreateStackInput{
			StackName:   aws.String(customDomainStackName(primaryDomain)),
			TemplateURL: aws.String(getReleaseUrl(customDomainFormationURL)),
			Parameters:  parameters,
			Tags:        cfnTags,
		}
		err = createStackOrChangeSet(sess, &input, createChangeSet, fmt.Sprintf("custom domain(s) %s", strings.Join(args, ", ")))
		checkErr(err)
	},
}

func init() {
	createCmd.AddCommand(createCustomDomainCmd)
}
