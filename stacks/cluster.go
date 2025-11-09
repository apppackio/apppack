package stacks

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
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

	var subnets []*net.IPNet
	subnets = append(subnets, &net.IPNet{IP: clusterCIDR.IP, Mask: subnetMask})
	prefix, _ := subnetMask.Size()

	for i := range 8 {
		nextCIDR, _ := cidr.NextSubnet(subnets[i], prefix)
		subnets = append(subnets, nextCIDR)
	}

	var publicSubnets []string
	for i := range 3 {
		publicSubnets = append(publicSubnets, subnets[i].String())
	}

	var privateSubnets []string
	for i := 6; i < 9; i++ {
		privateSubnets = append(privateSubnets, subnets[i].String())
	}

	return publicSubnets, privateSubnets, nil
}

// checkHostedZone prompts the user if the NS records for the domain don't match what AWS expects
func checkHostedZone(cfg aws.Config, zone *route53types.HostedZone) error {
	r53svc := route53.NewFromConfig(cfg)

	results, err := net.LookupNS(*zone.Name)
	if err != nil {
		return err
	}

	var actualServers []string
	for _, r := range results {
		actualServers = append(actualServers, strings.TrimSuffix(r.Host, "."))
	}

	var expectedServers []string

	resp, err := r53svc.GetHostedZone(context.Background(), &route53.GetHostedZoneInput{Id: zone.Id})
	if err != nil {
		return err
	}

	for _, ns := range resp.DelegationSet.NameServers {
		expectedServers = append(expectedServers, strings.TrimSuffix(ns, "."))
	}

	if hasSameItems(actualServers, expectedServers) {
		return nil
	}

	ui.Spinner.Stop()
	ui.PrintWarning(strings.TrimSuffix(*zone.Name, ".") + " doesn't appear to be using AWS' domain servers")
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

func (p *ClusterStackParameters) Import(parameters []types.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *ClusterStackParameters) ToCloudFormationParameters() ([]types.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *ClusterStackParameters) SetInternalFields(cfg aws.Config, _ *string) error {
	ui.StartSpinner()
	ui.Spinner.Suffix = " looking up hosted zone"

	zone, err := bridge.HostedZoneForDomain(cfg, p.Domain)
	if err != nil {
		return err
	}

	ui.Spinner.Suffix = " verifying DNS"

	if err = checkHostedZone(cfg, zone); err != nil {
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
		cfg.Region + "a",
		cfg.Region + "b",
		cfg.Region + "c",
	}

	ui.Spinner.Stop()

	return nil
}

type ClusterStack struct {
	Stack      *types.Stack
	Parameters *ClusterStackParameters
}

func (a *ClusterStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *ClusterStack) GetStack() *types.Stack {
	return a.Stack
}

func (a *ClusterStack) SetStack(stack *types.Stack) {
	a.Stack = stack
}

// SetDeletionProtection toggles the deletion protection flag on the load balancer
func (a *ClusterStack) SetDeletionProtection(cfg aws.Config, value bool) error {
	elbSvc := elasticloadbalancingv2.NewFromConfig(cfg)

	lbARN, err := bridge.GetStackOutput(a.Stack.Outputs, "LoadBalancerArn")
	if lbARN != nil {
		logrus.WithFields(logrus.Fields{"value": value}).Debug("setting load balancer deletion protection")
		_, err = elbSvc.ModifyLoadBalancerAttributes(context.Background(), &elasticloadbalancingv2.ModifyLoadBalancerAttributesInput{
			LoadBalancerArn: lbARN,
			Attributes: []elbv2types.LoadBalancerAttribute{
				{
					Key:   aws.String("deletion_protection.enabled"),
					Value: aws.String(strconv.FormatBool(value)),
				},
			},
		})

		return err
	}
	// if we get an error trying to set deletion protection, return it
	// just log errors trying to turn it off because the instance/cluster may not exist
	// in the case of a stack failure
	if err != nil {
		logrus.WithFields(logrus.Fields{"error": err}).Debug("unable to lookup Cloudformation outputs to set Load Balancer deletion protection")

		if value {
			return err
		}
	}

	return nil
}

func (a *ClusterStack) PostCreate(cfg aws.Config) error {
	return a.SetDeletionProtection(cfg, true)
}

func (a *ClusterStack) PreDelete(cfg aws.Config) error {
	return a.SetDeletionProtection(cfg, false)
}

func (*ClusterStack) PostDelete(_ aws.Config, _ *string) error {
	return nil
}

func (a *ClusterStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (a *ClusterStack) AskQuestions(_ aws.Config) error {
	var questions []*ui.QuestionExtra

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

func (*ClusterStack) Tags(name *string) []types.Tag {
	return []types.Tag{
		{Key: aws.String("apppack:cluster"), Value: name},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*ClusterStack) Capabilities() []types.Capability {
	return []types.Capability{
		types.CapabilityCapabilityIam,
	}
}

func (*ClusterStack) TemplateURL(release *string) *string {
	url := clusterFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}

	return &url
}
