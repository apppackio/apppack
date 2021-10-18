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
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/spf13/cobra"
)

func dedupe(s []string) []string {
	for i := 0; i < len(s); i++ {
		for i2 := i + 1; i2 < len(s); i2++ {
			if s[i] == s[i2] {
				// delete
				s = append(s[:i2], s[i2+1:]...)
				i2--
			}
		}
	}
	return s
}

// weight for ordering
var classOrder = []struct {
	Size   string
	Weight int
}{
	{"nano", 0},
	{"micro", 1},
	{"small", 2},
	{"medium", 3},
	{"large", 4},
	{"xlarge", 5},
	{"metal", 9999},
}

var previousGenerations = []string{
	"db.t2.",
	"db.m3.",
	"db.m4.",
	"db.r3.",
	"db.r4.",
}

func isPreviousGeneration(instanceClass *string) bool {
	for _, p := range previousGenerations {
		if strings.HasPrefix(*instanceClass, p) {
			return true
		}
	}
	return false
}

// instanceNameWeight creates a sortable string for instance classes
func instanceNameWeight(name string) string {
	parts := strings.Split(name, ".")
	var class string
	var size string
	// remove db. or cache. prefix
	if len(parts) == 3 {
		class = parts[1]
		size = parts[2]
	} else {
		class = parts[0]
		size = parts[1]
	}
	// extract multiplier (8xlarge) from size
	re := regexp.MustCompile("[0-9]+")
	multiplier := re.FindString(size)
	if multiplier != "" {
		num, err := strconv.Atoi(multiplier)
		if err != nil {
			return name
		}
		// multiply multiplier by 10 to bump it above the ones without multipliers
		return fmt.Sprintf("%s.%05d", class, num*10)
	}
	// determine string from static classOrder list
	for _, o := range classOrder {
		if size == o.Size {
			return fmt.Sprintf("%s.%05d", class, o.Weight)
		}
	}
	return name
}

func listRDSInstanceClasses(sess *session.Session, engine, version *string) ([]string, error) {
	rdsSvc := rds.New(sess)
	var instanceClassResults []*rds.OrderableDBInstanceOption

	err := rdsSvc.DescribeOrderableDBInstanceOptionsPages(&rds.DescribeOrderableDBInstanceOptionsInput{
		Engine:        engine,
		EngineVersion: version,
	}, func(resp *rds.DescribeOrderableDBInstanceOptionsOutput, lastPage bool) bool {
		for _, instanceOption := range resp.OrderableDBInstanceOptions {
			if instanceOption == nil {
				continue
			}

			instanceClassResults = append(instanceClassResults, instanceOption)
		}
		return !lastPage
	})
	if err != nil {
		return nil, err
	}
	var instanceClasses []string
	for _, opt := range instanceClassResults {
		if !isPreviousGeneration(opt.DBInstanceClass) {
			instanceClasses = append(instanceClasses, *opt.DBInstanceClass)
		}
	}
	instanceClasses = dedupe(instanceClasses)
	sort.Slice(instanceClasses, func(i int, j int) bool {
		return instanceNameWeight(instanceClasses[i]) < instanceNameWeight(instanceClasses[j])
	})
	return instanceClasses, nil
}

func engineName(engine *string, aurora bool) (*string, error) {
	if aurora {
		if *engine == "mysql" {
			return aws.String("aurora-mysql"), nil
		} else if *engine == "postgres" {
			return aws.String("aurora-postgresql"), nil
		} else {
			return nil, fmt.Errorf("unrecognized databae engine. valid options are 'mysql' or 'postgres'")
		}
	}
	if *engine == "mysql" || *engine == "postgres" {
		return engine, nil
	} else {
		return nil, fmt.Errorf("unrecognized databae engine. valid options are 'mysql' or 'postgres'")
	}
}

func getLatestRdsVersion(sess *session.Session, engine *string) (*string, error) {
	rdsSvc := rds.New(sess)
	resp, err := rdsSvc.DescribeDBEngineVersions(&rds.DescribeDBEngineVersionsInput{Engine: engine})
	if err != nil {
		return nil, err
	}
	return resp.DBEngineVersions[len(resp.DBEngineVersions)-1].EngineVersion, nil
}

// createDatabaseCmd represents the create database command
var createDatabaseCmd = &cobra.Command{
	Use:                   "database [<name>]",
	Short:                 "setup resources for an AppPack Database",
	Long:                  "*Requires admin permissions.*\nCreates an AppPack Database. If a `<name>` is not provided, the default name, `apppack` will be used.",
	DisableFlagsInUseLine: true,
	Args:                  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var name string
		if len(args) == 0 {
			name = "apppack"
		} else {
			name = args[0]
		}
		sess, err := adminSession()
		checkErr(err)
		var engine *string
		var isAurora bool
		var version *string
		answers := make(map[string]interface{})
		if !nonInteractive {
			questions := []*survey.Question{}
			clusterQuestion, err := makeClusterQuestion(sess, aws.String("AppPack Cluster to use for database"))
			checkErr(err)
			questions = append(questions, clusterQuestion)
			addQuestionFromFlag(cmd.Flags().Lookup("engine"), &questions, &survey.Question{
				Name:   "engine",
				Prompt: &survey.Select{Message: "select the database engine", Options: []string{"mysql", "postgres"}, FilterMessage: "", Default: "postgres"},
			})
			addQuestionFromFlag(cmd.Flags().Lookup("aurora"), &questions, nil)
			err = survey.Ask(questions, &answers)
			checkErr(err)
			engine = getArgValue(cmd, &answers, "engine", false)
			isAurora = isTruthy(getArgValue(cmd, &answers, "aurora", false))
			engine, err = engineName(engine, isAurora)
			checkErr(err)
			questions = []*survey.Question{}
			startSpinner()
			Spinner.Suffix = fmt.Sprintf(" retrieving instance classes for %s", *engine)
			version, err = getLatestRdsVersion(sess, engine)
			checkErr(err)
			instanceClasses, err := listRDSInstanceClasses(sess, engine, version)
			checkErr(err)
			Spinner.Stop()
			Spinner.Suffix = ""
			addQuestionFromFlag(cmd.Flags().Lookup("instance-class"), &questions, &survey.Question{
				Name:   "instance-class",
				Prompt: &survey.Select{Message: "select the instance class", Options: instanceClasses, FilterMessage: "", Default: "db.t3.medium"},
			})
			addQuestionFromFlag(cmd.Flags().Lookup("multi-az"), &questions, nil)
			if !isAurora {
				addQuestionFromFlag(cmd.Flags().Lookup("allocated-storage"), &questions, nil)
				addQuestionFromFlag(cmd.Flags().Lookup("max-allocated-storage"), &questions, nil)
			}
			err = survey.Ask(questions, &answers)
			checkErr(err)
		} else {
			engine = getArgValue(cmd, &answers, "engine", false)
			isAurora = isTruthy(getArgValue(cmd, &answers, "aurora", false))
			engine, err = engineName(engine, isAurora)
			checkErr(err)
			version, err = getLatestRdsVersion(sess, engine)
			checkErr(err)
		}
		cluster := getArgValue(cmd, &answers, "cluster", true)
		// check if a database already exists on the cluster
		_, err = getDDBClusterItem(sess, cluster, "DATABASE", &name)
		if err == nil {
			checkErr(fmt.Errorf(fmt.Sprintf("a database named %s already exists on the cluster %s", name, *cluster)))
		}
		clusterStack, err := stackFromDDBItem(sess, fmt.Sprintf("CLUSTER#%s", *cluster))
		checkErr(err)
		var multiAZParameter string
		if isTruthy((getArgValue(cmd, &answers, "multi-az", false))) {
			multiAZParameter = "yes"
		} else {
			multiAZParameter = "no"
		}

		if createChangeSet {
			fmt.Println("Creating Cloudformation Change Set for database resources...")
		} else {
			fmt.Println("Creating database resources, this may take a few minutes...")
		}
		startSpinner()
		cfnTags := []*cloudformation.Tag{
			{Key: aws.String("apppack:database"), Value: &name},
			{Key: aws.String("apppack:cluster"), Value: cluster},
			{Key: aws.String("apppack"), Value: aws.String("true")},
		}
		parameters := []*cloudformation.Parameter{
			{
				ParameterKey:   aws.String("Name"),
				ParameterValue: &name,
			},
			{
				ParameterKey:   aws.String("ClusterStackName"),
				ParameterValue: clusterStack.StackName,
			},
			{
				ParameterKey:   aws.String("Engine"),
				ParameterValue: engine,
			},
			{
				ParameterKey:   aws.String("Version"),
				ParameterValue: version,
			},
			{
				ParameterKey:   aws.String("OneTimePassword"),
				ParameterValue: aws.String(generatePassword()),
			},
			{
				ParameterKey:   aws.String("InstanceClass"),
				ParameterValue: getArgValue(cmd, &answers, "instance-class", true),
			},
			{
				ParameterKey:   aws.String("MultiAZ"),
				ParameterValue: &multiAZParameter,
			},
			{
				ParameterKey:   aws.String("AllocatedStorage"),
				ParameterValue: getArgValue(cmd, &answers, "allocated-storage", false),
			},
			{
				ParameterKey:   aws.String("MaxAllocatedStorage"),
				ParameterValue: getArgValue(cmd, &answers, "max-allocated-storage", false),
			},
		}

		input := cloudformation.CreateStackInput{
			StackName:    aws.String(fmt.Sprintf(databaseStackNameTmpl, name)),
			TemplateURL:  aws.String(getReleaseUrl(databaseFormationURL)),
			Parameters:   parameters,
			Capabilities: []*string{aws.String("CAPABILITY_IAM")},
			Tags:         cfnTags,
		}
		err = createStackOrChangeSet(sess, &input, createChangeSet, fmt.Sprintf("%s database", name))
		checkErr(err)
		if createChangeSet {
			printWarning(" deletion protection will not be enabled when the database is created. You can manually enable it after the database is created.")
			return
		}
		// enable deletion protection after create
		cfnSvc := cloudformation.New(sess)
		stackDesc, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: input.StackName,
		})
		checkErr(err)
		err = setRdsDeletionProtection(sess, stackDesc.Stacks[0], true)
		checkErr(err)
	},
}

func init() {
	createCmd.AddCommand(createDatabaseCmd)
	createDatabaseCmd.Flags().StringP("cluster", "c", "apppack", "cluster name")
	createDatabaseCmd.Flags().StringP("instance-class", "i", "db.t3.medium", "instance class -- see https://aws.amazon.com/rds/postgresql/pricing/?pg=pr&loc=3")
	createDatabaseCmd.Flags().BoolP("aurora", "a", false, "use Aurora -- see https://aws.amazon.com/rds/aurora/")
	createDatabaseCmd.Flags().StringP("engine", "e", "postgresql", "engine [mysql,postgres")
	createDatabaseCmd.Flags().Bool("multi-az", false, "enable multi-AZ -- see https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZ.html")
	createDatabaseCmd.Flags().Int("allocated-storage", 50, "initial storage allocated in GB (does not apply to Aurora engines)")
	createDatabaseCmd.Flags().Int("max-allocated-storage", 500, "maximum storage allocated on-demand in GB (does not apply to Aurora engines)")
}
