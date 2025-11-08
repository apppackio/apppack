package ddb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type stackItem struct {
	PrimaryID   string `json:"primary_id"`
	SecondaryID string `json:"secondary_id"`
	Stack       Stack  `json:"value"`
}

type Stack struct {
	StackID        string `json:"stack_id"`
	StackName      string `json:"stack_name"`
	Name           string `json:"name"`
	DatabaseEngine string `json:"engine"`
}

func GetClusterItem(sess *session.Session, cluster *string, addon string, name *string) (*Stack, error) {
	ddbSvc := dynamodb.New(sess)
	secondaryID := fmt.Sprintf("%s#%s#%s", *cluster, addon, *name)

	result, err := ddbSvc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("apppack"),
		Key: map[string]*dynamodb.AttributeValue{
			"primary_id": {
				S: aws.String("CLUSTERS"),
			},
			"secondary_id": {
				S: aws.String(secondaryID),
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

	err = dynamodbattribute.UnmarshalMap(result.Item, &i)
	if err != nil {
		return nil, err
	}

	return &i.Stack, nil
}

func ClusterQuery(sess *session.Session, cluster, addon *string) (*[]map[string]*dynamodb.AttributeValue, error) {
	ddbSvc := dynamodb.New(sess)

	result, err := ddbSvc.Query(&dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1 AND begins_with(secondary_id,:id2)"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id1": {S: aws.String("CLUSTERS")},
			":id2": {S: aws.String(fmt.Sprintf("%s#%s#", *cluster, *addon))},
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

func ListStacks(sess *session.Session, cluster *string, addon string) ([]string, error) {
	items, err := ClusterQuery(sess, cluster, &addon)
	if err != nil {
		return nil, err
	}

	var i []stackItem

	err = dynamodbattribute.UnmarshalListOfMaps(*items, &i)
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

func ListClusters(sess *session.Session) ([]string, error) {
	ddbSvc := dynamodb.New(sess)

	result, err := ddbSvc.Query(&dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1 AND begins_with(secondary_id,:id2)"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id1": {S: aws.String("CLUSTERS")},
			":id2": {S: aws.String("CLUSTER#")},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Items == nil {
		return nil, errors.New("could not find any AppPack clusters")
	}

	var i []stackItem

	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &i)
	if err != nil {
		return nil, err
	}

	var clusters []string

	for idx := range i {
		clusters = append(clusters, i[idx].Stack.Name)
	}

	return clusters, nil
}

func StackFromItem(sess *session.Session, secondaryID string) (*cloudformation.Stack, error) {
	ddbSvc := dynamodb.New(sess)

	result, err := ddbSvc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("apppack"),
		Key: map[string]*dynamodb.AttributeValue{
			"primary_id": {
				S: aws.String("CLUSTERS"),
			},
			"secondary_id": {
				S: aws.String(secondaryID),
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

	err = dynamodbattribute.UnmarshalMap(result.Item, &i)
	if err != nil {
		return nil, err
	}

	cfnSvc := cloudformation.New(sess)

	stacks, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &i.Stack.StackID,
	})
	if err != nil {
		return nil, err
	}

	if len(stacks.Stacks) == 0 {
		return nil, fmt.Errorf("no stacks found with ID %s", i.Stack.StackID)
	}

	return stacks.Stacks[0], nil
}
