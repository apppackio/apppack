package ddb

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

type Stack struct {
	StackID        string `dynamodbav:"stack_id"`
	StackName      string `dynamodbav:"stack_name"`
	Name           string `dynamodbav:"name"`
	DatabaseEngine string `dynamodbav:"engine"`
}

func GetClusterItem(cfg aws.Config, cluster *string, addon string, name *string) (*Stack, error) {
	ddbSvc := dynamodb.NewFromConfig(cfg)
	secondaryID := fmt.Sprintf("%s#%s#%s", *cluster, addon, *name)

	result, err := ddbSvc.GetItem(context.Background(), &dynamodb.GetItemInput{
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
	ddbSvc := dynamodb.NewFromConfig(cfg)

	result, err := ddbSvc.Query(context.Background(), &dynamodb.QueryInput{
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

	if result.Items == nil {
		return nil, fmt.Errorf("could not find any AppPack %s stacks on %s cluster", strings.ToLower(*addon), *cluster)
	}

	return &result.Items, nil
}

func ListStacks(cfg aws.Config, cluster *string, addon string) ([]string, error) {
	items, err := ClusterQuery(cfg, cluster, &addon)
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
	ddbSvc := dynamodb.NewFromConfig(cfg)

	result, err := ddbSvc.Query(context.Background(), &dynamodb.QueryInput{
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

	if result.Items == nil {
		return nil, errors.New("could not find any AppPack clusters")
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
	ddbSvc := dynamodb.NewFromConfig(cfg)

	result, err := ddbSvc.GetItem(context.Background(), &dynamodb.GetItemInput{
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

	cfnSvc := cloudformation.NewFromConfig(cfg)

	stacks, err := cfnSvc.DescribeStacks(context.Background(), &cloudformation.DescribeStacksInput{
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
