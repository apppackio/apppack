package stacks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

var previousRDSGenerations = []string{
	"db.t2.",
	"db.t3",
	"db.m3.",
	"db.m4.",
	"db.m5.",
	"db.m5d.",
	"db.r3.",
	"db.r4.",
	"db.r5.",
	"db.r5b.",
	"db.r5d.",
}

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

func isPreviousRDSGeneration(instanceClass *string) bool {
	for _, p := range previousRDSGenerations {
		if strings.HasPrefix(*instanceClass, p) {
			return true
		}
	}
	return false
}

func auroraEngineName(engine *string) (string, error) {
	if *engine == "mysql" {
		return "aurora-mysql", nil
	}
	if *engine == "postgres" {
		return "aurora-postgresql", nil
	}
	return "", fmt.Errorf("unrecognized databae engine. valid options are 'mysql' or 'postgres'")
}

func standardEngineName(engine *string) (string, error) {
	if *engine == "mysql" || *engine == "postgres" {
		return *engine, nil
	}
	return "", fmt.Errorf("unrecognized databae engine. valid options are 'mysql' or 'postgres'")
}

func getLatestRdsVersion(sess *session.Session, engine *string) (string, error) {
	rdsSvc := rds.New(sess)
	resp, err := rdsSvc.DescribeDBEngineVersions(&rds.DescribeDBEngineVersionsInput{Engine: engine})
	if err != nil {
		return "", err
	}
	// Filter for the latest version without "limitless"
	for i := len(resp.DBEngineVersions) - 1; i >= 0; i-- {
		if version := *resp.DBEngineVersions[i].EngineVersion; !strings.Contains(version, "limitless") {
			return version, nil
		}
	}
	return "", fmt.Errorf("no compatible version found for engine %s", *engine)
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
		if !isPreviousRDSGeneration(opt.DBInstanceClass) && *opt.DBInstanceClass != "db.serverless" {
			instanceClasses = append(instanceClasses, *opt.DBInstanceClass)
		}
	}
	instanceClasses = dedupe(instanceClasses)
	bridge.SortInstanceClasses(instanceClasses)
	return instanceClasses, nil
}

type DatabaseStackParameters struct {
	Name                string
	ClusterStackName    string `flag:"cluster;fmtString:apppack-cluster-%s"`
	Engine              string `flag:"engine"`
	Version             string
	OneTimePassword     string
	InstanceClass       string `flag:"instance-class"`
	MultiAZ             bool   `flag:"multi-az" cfnbool:"yesno"`
	AllocatedStorage    int    `flag:"allocated-storage"`
	MaxAllocatedStorage int    `flag:"max-allocated-storage"`
}

func (p *DatabaseStackParameters) Import(parameters []*cloudformation.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *DatabaseStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *DatabaseStackParameters) SetInternalFields(sess *session.Session, name *string) error {
	var err error
	if p.Version == "" {
		p.Version, err = getLatestRdsVersion(sess, &p.Engine)
		if err != nil {
			return err
		}
	}
	p.OneTimePassword, err = GeneratePassword()
	if err != nil {
		return err
	}
	if p.Name == "" {
		p.Name = *name
	}
	return nil
}

var DefaultDatabaseStackParameters = DatabaseStackParameters{
	Engine:              "postgres",
	MultiAZ:             false,
	MaxAllocatedStorage: 500,
	AllocatedStorage:    50,
	InstanceClass:       "db.t4g.medium",
}

type DatabaseStack struct {
	Stack      *cloudformation.Stack
	Parameters *DatabaseStackParameters
}

func (a *DatabaseStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *DatabaseStack) GetStack() *cloudformation.Stack {
	return a.Stack
}

func (a *DatabaseStack) SetStack(stack *cloudformation.Stack) {
	a.Stack = stack
}

// SetDeletionProtection toggles the deletion protection flag on the database instance or cluster
func (a *DatabaseStack) SetDeletionProtection(sess *session.Session, value bool) error {
	rdsSvc := rds.New(sess)
	DBID, err1 := bridge.GetStackOutput(a.Stack.Outputs, "DBId")
	DBType, err2 := bridge.GetStackOutput(a.Stack.Outputs, "DBType")
	// If stack failed to complete successfully, we may not have a DB instance to modify
	if DBID != nil && DBType != nil {
		input := rds.ModifyDBInstanceInput{
			DBInstanceIdentifier: DBID,
			DeletionProtection:   &value,
			ApplyImmediately:     aws.Bool(true),
		}

		logrus.WithFields(logrus.Fields{"identifier": DBID, "value": value}).Debug("setting RDS deletion protection")
		if *DBType == "instance" {
			_, err := rdsSvc.ModifyDBInstance(&input)

			return err
		}
		if *DBType == "cluster" {
			_, err := rdsSvc.ModifyDBCluster(&rds.ModifyDBClusterInput{
				DBClusterIdentifier: input.DBInstanceIdentifier,
				DeletionProtection:  input.DeletionProtection,
				ApplyImmediately:    input.ApplyImmediately,
			})
			return err
		}
		return fmt.Errorf("unexpected DB type %s", *DBType)
	}
	// if we get an error trying to set deletion protection, return it
	// just log errors trying to turn it off because the instance/cluster may not exist
	// in the case of a stack failure
	if err1 != nil {
		logrus.WithFields(
			logrus.Fields{"error": err1},
		).Debug("unable to lookup Cloudformation outputs to set RDS deletion protection")
		if value {
			return err1
		}
	}
	if err2 != nil {
		logrus.WithFields(
			logrus.Fields{"error": err2},
		).Debug("unable to lookup Cloudformation outputs to set RDS deletion protection")
		if value {
			return err2
		}
	}
	return nil
}

// PreDelete will remove deletion protection on the stack
func (a *DatabaseStack) PreDelete(sess *session.Session) error {
	return a.SetDeletionProtection(sess, false)
}

func (a *DatabaseStack) PostCreate(sess *session.Session) error {
	return a.SetDeletionProtection(sess, true)
}

func (*DatabaseStack) PostDelete(_ *session.Session, _ *string) error {
	return nil
}

func (a *DatabaseStack) ClusterName() string {
	return strings.TrimPrefix(a.Parameters.ClusterStackName, fmt.Sprintf(clusterStackNameTmpl, ""))
}

func (a *DatabaseStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (a *DatabaseStack) AskQuestions(sess *session.Session) error {
	var questions []*ui.QuestionExtra
	var err error
	var aurora bool
	var auroraSel string = ui.BooleanAsYesNo(aurora)
	if a.Stack == nil {
		err = AskForCluster(
			sess,
			"Which cluster should this Database be installed in?",
			"A cluster represents an isolated network and its associated resources (Apps, Database, Redis, etc.).",
			a.Parameters,
		)
		if err != nil {
			return err
		}

		questions = append(questions, []*ui.QuestionExtra{
			{
				Verbose:  "What engine should this Database use?",
				HelpText: "",
				Form: huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Type").
							Options(huh.NewOptions("postgres", "mysql")...).
							Value(&a.Parameters.Engine),
					),
				),
			},
			{
				Verbose:  "Should this Database use the Aurora engine variant?",
				HelpText: "Aurora provides many benefits over the standard engines, but is not available on very small instance sizes. For more info see https://aws.amazon.com/rds/aurora/.",
				WriteTo:  &ui.BooleanOptionProxy{Value: &aurora},
				Form: huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Aurora").
							Options(huh.NewOptions("yes", "no")...).
							Value(&auroraSel),
					),
				),
			},
		}...)
		if err = ui.AskQuestions(questions, a.Parameters); err != nil {
			return err
		}
		ui.StartSpinner()
		if aurora {
			a.Parameters.Engine, err = auroraEngineName(&a.Parameters.Engine)
		} else {
			a.Parameters.Engine, err = standardEngineName(&a.Parameters.Engine)
		}
		if err != nil {
			return err
		}
		a.Parameters.Version, err = getLatestRdsVersion(sess, &a.Parameters.Engine)
		if err != nil {
			return err
		}
		questions = []*ui.QuestionExtra{}
	}
	ui.StartSpinner()
	ui.Spinner.Suffix = fmt.Sprintf(" retrieving instance classes for %s", a.Parameters.Engine)
	instanceClasses, err := listRDSInstanceClasses(sess, &a.Parameters.Engine, &a.Parameters.Version)
	if err != nil {
		return err
	}
	ui.Spinner.Stop()
	ui.Spinner.Suffix = ""
	var multiAZSel string = ui.BooleanAsYesNo(a.Parameters.MultiAZ)
	questions = append(questions, []*ui.QuestionExtra{
		{
			Verbose:  "What instance class should be used for this Database?",
			HelpText: "Enter the Database instance class. For more info see https://aws.amazon.com/rds/pricing/.",
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Instance Class").
						Options(huh.NewOptions(instanceClasses...)...).
						Value(&a.Parameters.InstanceClass),
				),
			),
		},
		{
			Verbose: "Should this Database be setup in multiple availability zones?",
			HelpText: "Multiple availability zones (AZs) provide more resilience in the case of an AZ outage, " +
				"but double the cost at AWS. In the case of Aurora databases, enabling multiple availability zones will give you access to a read-replica." +
				"For more info see https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZ.html.",
			WriteTo: &ui.BooleanOptionProxy{Value: &a.Parameters.MultiAZ},
			Form: huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Multi AZ").
						Options(huh.NewOptions("yes", "no")...).
						Value(&multiAZSel),
				),
			),
		},
	}...)
	return ui.AskQuestions(questions, a.Parameters)
}

func (*DatabaseStack) StackName(name *string) *string {
	stackName := fmt.Sprintf(databaseStackNameTmpl, *name)

	return &stackName
}

func (*DatabaseStack) StackType() string {
	return "database"
}

func (a *DatabaseStack) Tags(name *string) []*cloudformation.Tag {
	return []*cloudformation.Tag{
		{Key: aws.String("apppack:database"), Value: name},
		{Key: aws.String("apppack:cluster"), Value: aws.String(a.ClusterName())},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*DatabaseStack) Capabilities() []*string {
	return []*string{
		aws.String("CAPABILITY_IAM"),
	}
}

func (*DatabaseStack) TemplateURL(release *string) *string {
	url := databaseFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}
	return &url
}
