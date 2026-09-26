package ddb

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// AWS SDK v2's `attributevalue` package only honors `dynamodbav` tags. SDK v1's
// `dynamodbattribute` honored `json` tags as a fallback, which masked the
// missing tags here until the v2 migration in #106. Without these, all
// UnmarshalMap / UnmarshalListOfMaps calls return zero-valued structs.
type stackItem struct {
	PrimaryID   string `dynamodbav:"primary_id"`
	SecondaryID string `dynamodbav:"secondary_id"`
	Stack       Stack  `dynamodbav:"value"`
}

// Each of these names just the one method its callers use, so the AWS SDK's
// generated clients already satisfy them and a test fake is a plain struct
// with one method.
type (
	itemGetter interface {
		GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	}

	querier interface {
		Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	}

	stackDescriber interface {
		DescribeStacks(context.Context, *cloudformation.DescribeStacksInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
	}
)

type Stack struct {
	StackID        string `dynamodbav:"stack_id"`
	StackName      string `dynamodbav:"stack_name"`
	Name           string `dynamodbav:"name"`
	DatabaseEngine string `dynamodbav:"engine"`
}

func GetClusterItem(cfg aws.Config, cluster *string, addon string, name *string) (*Stack, error) {
	return getClusterItem(context.Background(), dynamodb.NewFromConfig(cfg), cluster, addon, name)
}

func getClusterItem(ctx context.Context, ddbSvc itemGetter, cluster *string, addon string, name *string) (*Stack, error) {
	secondaryID := fmt.Sprintf("%s#%s#%s", *cluster, addon, *name)

	result, err := ddbSvc.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String("apppack"),
		Key: map[string]dynamodbtypes.AttributeValue{
			"primary_id": &dynamodbtypes.AttributeValueMemberS{
				Value: "CLUSTERS",
			},
			"secondary_id": &dynamodbtypes.AttributeValueMemberS{
				Value: secondaryID,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, fmt.Errorf("could not find CLUSTERS/%s", secondaryID)
	}

	i := stackItem{}

	err = attributevalue.UnmarshalMap(result.Item, &i)
	if err != nil {
		return nil, err
	}

	return &i.Stack, nil
}

func ClusterQuery(cfg aws.Config, cluster, addon *string) (*[]map[string]dynamodbtypes.AttributeValue, error) {
	return clusterQuery(context.Background(), dynamodb.NewFromConfig(cfg), cluster, addon)
}

func clusterQuery(ctx context.Context, ddbSvc querier, cluster, addon *string) (*[]map[string]dynamodbtypes.AttributeValue, error) {
	result, err := ddbSvc.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1 AND begins_with(secondary_id,:id2)"),
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":id1": &dynamodbtypes.AttributeValueMemberS{Value: "CLUSTERS"},
			":id2": &dynamodbtypes.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s#", *cluster, *addon)},
		},
	})
	if err != nil {
		return nil, err
	}

	// No matches is not an error here. Callers say it better:
	// selectDatabaseStack points at `apppack create database`, and
	// AskForCluster at "no AppPack clusters are setup".
	return &result.Items, nil
}

func ListStacks(cfg aws.Config, cluster *string, addon string) ([]string, error) {
	return listStacks(context.Background(), dynamodb.NewFromConfig(cfg), cluster, addon)
}

func listStacks(ctx context.Context, ddbSvc querier, cluster *string, addon string) ([]string, error) {
	items, err := clusterQuery(ctx, ddbSvc, cluster, &addon)
	if err != nil {
		return nil, err
	}

	var i []stackItem

	err = attributevalue.UnmarshalListOfMaps(*items, &i)
	if err != nil {
		return nil, err
	}

	var (
		stacks []string
		stack  Stack
	)

	for idx := range i {
		stack = i[idx].Stack
		if len(stack.DatabaseEngine) > 0 {
			stacks = append(stacks, fmt.Sprintf("%s (%s)", stack.Name, stack.DatabaseEngine))
		} else {
			stacks = append(stacks, stack.Name)
		}
	}

	return stacks, nil
}

func ListClusters(cfg aws.Config) ([]string, error) {
	return listClusters(context.Background(), dynamodb.NewFromConfig(cfg))
}

func listClusters(ctx context.Context, ddbSvc querier) ([]string, error) {
	result, err := ddbSvc.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1 AND begins_with(secondary_id,:id2)"),
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":id1": &dynamodbtypes.AttributeValueMemberS{Value: "CLUSTERS"},
			":id2": &dynamodbtypes.AttributeValueMemberS{Value: "CLUSTER#"},
		},
	})
	if err != nil {
		return nil, err
	}

	var i []stackItem

	err = attributevalue.UnmarshalListOfMaps(result.Items, &i)
	if err != nil {
		return nil, err
	}

	var clusters []string

	for idx := range i {
		clusters = append(clusters, i[idx].Stack.Name)
	}

	return clusters, nil
}

func StackFromItem(cfg aws.Config, secondaryID string) (*types.Stack, error) {
	return stackFromItem(context.Background(), dynamodb.NewFromConfig(cfg), cloudformation.NewFromConfig(cfg), secondaryID)
}

func stackFromItem(ctx context.Context, ddbSvc itemGetter, cfnSvc stackDescriber, secondaryID string) (*types.Stack, error) {
	result, err := ddbSvc.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String("apppack"),
		Key: map[string]dynamodbtypes.AttributeValue{
			"primary_id": &dynamodbtypes.AttributeValueMemberS{
				Value: "CLUSTERS",
			},
			"secondary_id": &dynamodbtypes.AttributeValueMemberS{
				Value: secondaryID,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, fmt.Errorf("could not find CLUSTERS/%s", secondaryID)
	}

	i := stackItem{}

	err = attributevalue.UnmarshalMap(result.Item, &i)
	if err != nil {
		return nil, err
	}

	stacks, err := cfnSvc.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &i.Stack.StackID,
	})
	if err != nil {
		return nil, err
	}

	if len(stacks.Stacks) == 0 {
		return nil, fmt.Errorf("no stacks found with ID %s", i.Stack.StackID)
	}

	return &stacks.Stacks[0], nil
}
