package ddb

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStackItemUnmarshal asserts that a DDB item matching what apppack writes
// for a cluster registration unmarshals correctly into stackItem.
//
// Regression guard for the SDK v1→v2 migration (#106): SDK v2's attributevalue
// package only honors `dynamodbav` tags. The struct previously used `json`
// tags, which SDK v1 honored as a fallback but SDK v2 does not. The result
// was every DDB read returning zero-valued structs.
func TestStackItemUnmarshal(t *testing.T) {
	// Shape mirrors what the cluster CloudFormation custom resource writes to
	// the apppack DDB table: see formations/cluster/cluster.py — ClusterDdbItem.
	item := map[string]dynamodbtypes.AttributeValue{
		"primary_id":   &dynamodbtypes.AttributeValueMemberS{Value: "CLUSTERS"},
		"secondary_id": &dynamodbtypes.AttributeValueMemberS{Value: "CLUSTER#apppack"},
		"value": &dynamodbtypes.AttributeValueMemberM{
			Value: map[string]dynamodbtypes.AttributeValue{
				"name":       &dynamodbtypes.AttributeValueMemberS{Value: "apppack"},
				"stack_name": &dynamodbtypes.AttributeValueMemberS{Value: "apppack-cluster-apppack"},
				"stack_id":   &dynamodbtypes.AttributeValueMemberS{Value: "arn:aws:cloudformation:us-east-1:123456789012:stack/apppack-cluster-apppack/abc-def"},
			},
		},
	}

	var got stackItem
	require.NoError(t, attributevalue.UnmarshalMap(item, &got))

	assert.Equal(t, "CLUSTERS", got.PrimaryID)
	assert.Equal(t, "CLUSTER#apppack", got.SecondaryID)
	assert.Equal(t, "apppack", got.Stack.Name)
	assert.Equal(t, "apppack-cluster-apppack", got.Stack.StackName)
	assert.Equal(t, "arn:aws:cloudformation:us-east-1:123456789012:stack/apppack-cluster-apppack/abc-def", got.Stack.StackID)
	assert.Empty(t, got.Stack.DatabaseEngine, "DatabaseEngine should be empty when not present in DDB")
}

// TestStackItemUnmarshalWithEngine covers the database/redis case where the
// `engine` field is populated.
func TestStackItemUnmarshalWithEngine(t *testing.T) {
	item := map[string]dynamodbtypes.AttributeValue{
		"primary_id":   &dynamodbtypes.AttributeValueMemberS{Value: "CLUSTERS"},
		"secondary_id": &dynamodbtypes.AttributeValueMemberS{Value: "CLUSTER#apppack#DATABASE#sandbox-db"},
		"value": &dynamodbtypes.AttributeValueMemberM{
			Value: map[string]dynamodbtypes.AttributeValue{
				"name":       &dynamodbtypes.AttributeValueMemberS{Value: "sandbox-db"},
				"stack_name": &dynamodbtypes.AttributeValueMemberS{Value: "apppack-database-sandbox-db"},
				"stack_id":   &dynamodbtypes.AttributeValueMemberS{Value: "arn:aws:cloudformation:us-east-1:123456789012:stack/apppack-database-sandbox-db/x"},
				"engine":     &dynamodbtypes.AttributeValueMemberS{Value: "postgres"},
			},
		},
	}

	var got stackItem
	require.NoError(t, attributevalue.UnmarshalMap(item, &got))

	assert.Equal(t, "sandbox-db", got.Stack.Name)
	assert.Equal(t, "postgres", got.Stack.DatabaseEngine)
}
