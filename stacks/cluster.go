package stacks

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

// splitSubnet takes a CIDR string and splits it up into two groups of 3 comma-separated subnets
func splitSubnet(cidrStr string) ([]string, []string, error) {
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
	return publicSubnets, privateSubnets, nil
}

// checkHostedZone prompts the user if the NS records for the domain don't match what AWS expects
func checkHostedZone(sess *session.Session, zone *route53.HostedZone) error {
	r53svc := route53.New(sess)
	results, err := net.LookupNS(*zone.Name)
	if err != nil {
		return err
	}
	actualServers := []string{}
	for _, r := range results {
		actualServers = append(actualServers, strings.TrimSuffix(r.Host, "."))
	}
	expectedServers := []string{}
	resp, err := r53svc.GetHostedZone(&route53.GetHostedZoneInput{Id: zone.Id})
	if err != nil {
		return err
	}
	for _, ns := range resp.DelegationSet.NameServers {
		expectedServers = append(expectedServers, strings.TrimSuffix(*ns, "."))
	}
	if hasSameItems(actualServers, expectedServers) {
		return nil
	}
	ui.Spinner.Stop()
	ui.PrintWarning(fmt.Sprintf("%s doesn't appear to be using AWS' domain servers", strings.TrimSuffix(*zone.Name, ".")))
	fmt.Printf("Expected:\n  %s\n\n", strings.Join(expectedServers, "\n  "))
	fmt.Printf("Actual:\n  %s\n\n", strings.Join(actualServers, "\n  "))
	fmt.Printf("If nameservers are not setup properly, TLS certificate creation will fail.\n")
	ui.PauseUntilEnter("Once you've verified the nameservers are correct, press ENTER to continue.")
	return nil
}

// hasSameItems verifies two string slices contain the same elements
func hasSameItems(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

type ClusterStackParameters struct {
	Name               string
	Cidr               string `flag:"cidr"`
	PublicSubnetCidrs  []string
	PrivateSubnetCidrs []string
	AvailabilityZones  []string
	InstanceType       string
	Domain             string `flag:"domain"`
	HostedZone         string
}

var DefaultClusterStackParameters = ClusterStackParameters{Cidr: "10.100.0.0/16"}

func (p *ClusterStackParameters) Import(parameters []*cloudformation.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *ClusterStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *ClusterStackParameters) SetInternalFields(sess *session.Session, _ *string) error {
	ui.StartSpinner()
	ui.Spinner.Suffix = " looking up hosted zone"
	zone, err := bridge.HostedZoneForDomain(sess, p.Domain)
	if err != nil {
		return err
	}
	ui.Spinner.Suffix = " verifying DNS"
	if err = checkHostedZone(sess, zone); err != nil {
		return err
	}
	p.HostedZone = strings.Split(*zone.Id, "/")[2]
	ui.Spinner.Suffix = ""
	publicSubnets, privateSubnets, err := splitSubnet(p.Cidr)
	if err != nil {
		return err
	}
	if len(p.PrivateSubnetCidrs) == 0 {
		p.PrivateSubnetCidrs = privateSubnets
	}
	if len(p.PublicSubnetCidrs) == 0 {
		p.PublicSubnetCidrs = publicSubnets
	}
	p.AvailabilityZones = []string{
		fmt.Sprintf("%sa", *sess.Config.Region),
		fmt.Sprintf("%sb", *sess.Config.Region),
		fmt.Sprintf("%sc", *sess.Config.Region),
	}
	ui.Spinner.Stop()
	return nil
}

type ClusterStack struct {
	Stack      *cloudformation.Stack
	Parameters *ClusterStackParameters
}

func (a *ClusterStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *ClusterStack) GetStack() *cloudformation.Stack {
	return a.Stack
}

func (a *ClusterStack) SetStack(stack *cloudformation.Stack) {
	a.Stack = stack
}

// SetDeletionProtection toggles the deletion protection flag on the load balancer
func (a *ClusterStack) SetDeletionProtection(sess *session.Session, value bool) error {
	elbSvc := elbv2.New(sess)
	lbARN, err := bridge.GetStackOutput(a.Stack.Outputs, "LoadBalancerArn")
	if err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{"value": value}).Debug("setting load balancer deletion protection")
	_, err = elbSvc.ModifyLoadBalancerAttributes(&elbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: lbARN,
		Attributes: []*elbv2.LoadBalancerAttribute{
			{
				Key:   aws.String("deletion_protection.enabled"),
				Value: aws.String(strconv.FormatBool(value)),
			},
		},
	})
	return err
}

func (a *ClusterStack) PostCreate(sess *session.Session) error {
	return a.SetDeletionProtection(sess, true)
}

func (a *ClusterStack) PreDelete(sess *session.Session) error {
	return a.SetDeletionProtection(sess, false)
}

func (*ClusterStack) PostDelete(_ *session.Session, _ *string) error {
	return nil
}

func (a *ClusterStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (a *ClusterStack) AskQuestions(_ *session.Session) error {
	questions := []*ui.QuestionExtra{}
	var err error
	if a.Stack == nil {
		questions = append(questions, []*ui.QuestionExtra{
			{
				Verbose:  "What domain should be associated with this cluster?",
				HelpText: "Apps installed to this cluster will automatically get assigned a subdomain on the provided domain. The domain or a parent domain must already be setup as a Route53 Hosted Zone. See https://docs.apppack.io/how-to/bring-your-own-cluster-domain/ for more info.",
				Question: &survey.Question{
					Name:     "Domain",
					Prompt:   &survey.Input{Message: "Cluster Domain", Default: a.Parameters.Domain},
					Validate: survey.Required,
				},
			},
		}...)
		if err = ui.AskQuestions(questions, a.Parameters); err != nil {
			return err
		}
	}
	return nil
}

func (*ClusterStack) StackName(name *string) *string {
	stackName := fmt.Sprintf(clusterStackNameTmpl, *name)
	return &stackName
}

func (*ClusterStack) StackType() string {
	return "cluster"
}

func (*ClusterStack) Tags(name *string) []*cloudformation.Tag {
	return []*cloudformation.Tag{
		{Key: aws.String("apppack:cluster"), Value: name},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*ClusterStack) Capabilities() []*string {
	return []*string{
		aws.String("CAPABILITY_IAM"),
	}
}

func (*ClusterStack) TemplateURL(release *string) *string {
	url := clusterFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}
	return &url
}
