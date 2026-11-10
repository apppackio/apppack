package ddb

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
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

// The SDK's generated clients already satisfy the interfaces in ddb.go, so
// these fakes are plain structs -- no mock framework.

type fakeQuerier struct {
	out *dynamodb.QueryOutput
	err error
	in  *dynamodb.QueryInput
}

func (f *fakeQuerier) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.in = in

	return f.out, f.err
}

type fakeItemGetter struct {
	out *dynamodb.GetItemOutput
	err error
	in  *dynamodb.GetItemInput
}

func (f *fakeItemGetter) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.in = in

	return f.out, f.err
}

type fakeStackDescriber struct {
	out *cloudformation.DescribeStacksOutput
	err error
}

func (f *fakeStackDescriber) DescribeStacks(context.Context, *cloudformation.DescribeStacksInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
	return f.out, f.err
}

// stringAttr reads a string attribute back out, failing the test rather than
// panicking if the shape is wrong.
func stringAttr(t *testing.T, v dynamodbtypes.AttributeValue) string {
	t.Helper()

	s, ok := v.(*dynamodbtypes.AttributeValueMemberS)
	if !ok {
		t.Fatalf("attribute is %T, want a string", v)
	}

	return s.Value
}

func str(v string) dynamodbtypes.AttributeValue {
	return &dynamodbtypes.AttributeValueMemberS{Value: v}
}

// stackAttr builds the "value" map as the cluster CloudFormation custom
// resource writes it.
func stackAttr(name, stackID, engine string) dynamodbtypes.AttributeValue {
	m := map[string]dynamodbtypes.AttributeValue{
		"name":     str(name),
		"stack_id": str(stackID),
	}
	if engine != "" {
		m["engine"] = str(engine)
	}

	return &dynamodbtypes.AttributeValueMemberM{Value: m}
}

func item(secondaryID, name, stackID, engine string) map[string]dynamodbtypes.AttributeValue {
	return map[string]dynamodbtypes.AttributeValue{
		"primary_id":   str("CLUSTERS"),
		"secondary_id": str(secondaryID),
		"value":        stackAttr(name, stackID, engine),
	}
}

// TestClusterQueryNoMatchesIsNotAnError -- "nothing on this cluster" is the
// caller's call to make, not ours. selectDatabaseStack answers it with
// "create one first with `apppack create database`"; an error raised here
// would pre-empt that with a worse message.
func TestClusterQueryNoMatchesIsNotAnError(t *testing.T) {
	t.Parallel()

	cluster, addon := "prod", "DATABASE"

	for name, items := range map[string][]map[string]dynamodbtypes.AttributeValue{
		"empty slice": {},
		"nil slice":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc := &fakeQuerier{out: &dynamodb.QueryOutput{Items: items}}

			got, err := clusterQuery(context.Background(), svc, &cluster, &addon)
			if err != nil {
				t.Fatalf("clusterQuery: %v", err)
			}

			if len(*got) != 0 {
				t.Errorf("got %d items, want none", len(*got))
			}
		})
	}
}

func TestClusterQueryBuildsTheKeyCondition(t *testing.T) {
	t.Parallel()

	svc := &fakeQuerier{out: &dynamodb.QueryOutput{}}
	cluster, addon := "prod", "DATABASE"

	if _, err := clusterQuery(context.Background(), svc, &cluster, &addon); err != nil {
		t.Fatalf("clusterQuery: %v", err)
	}

	if *svc.in.TableName != "apppack" {
		t.Errorf("TableName = %q", *svc.in.TableName)
	}

	prefix := stringAttr(t, svc.in.ExpressionAttributeValues[":id2"])
	if prefix != "prod#DATABASE#" {
		t.Errorf("secondary_id prefix = %q, want prod#DATABASE#", prefix)
	}
}

func TestClusterQueryPropagatesError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("throughput exceeded")
	svc := &fakeQuerier{err: sentinel}
	cluster, addon := "prod", "DATABASE"

	_, err := clusterQuery(context.Background(), svc, &cluster, &addon)
	if !errors.Is(err, sentinel) {
		t.Errorf("got %v, want the underlying error", err)
	}
}

func TestListStacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []map[string]dynamodbtypes.AttributeValue
		want  []string
	}{
		{
			// The engine suffix is what `create app` strips back off in
			// selectDatabaseStack, so the format matters on both sides.
			name:  "database entries carry the engine",
			items: []map[string]dynamodbtypes.AttributeValue{item("prod#DATABASE#main", "main", "arn:1", "postgres")},
			want:  []string{"main (postgres)"},
		},
		{
			name:  "entries without an engine are bare names",
			items: []map[string]dynamodbtypes.AttributeValue{item("prod#REDIS#cache", "cache", "arn:2", "")},
			want:  []string{"cache"},
		},
		{
			name: "order is preserved",
			items: []map[string]dynamodbtypes.AttributeValue{
				item("prod#DATABASE#b", "b", "arn:1", "mysql"),
				item("prod#DATABASE#a", "a", "arn:2", ""),
			},
			want: []string{"b (mysql)", "a"},
		},
		{
			name:  "no stacks",
			items: []map[string]dynamodbtypes.AttributeValue{},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &fakeQuerier{out: &dynamodb.QueryOutput{Items: tt.items}}
			cluster := "prod"

			got, err := listStacks(context.Background(), svc, &cluster, "DATABASE")
			if err != nil {
				t.Fatalf("listStacks: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestListClusters(t *testing.T) {
	t.Parallel()

	svc := &fakeQuerier{out: &dynamodb.QueryOutput{Items: []map[string]dynamodbtypes.AttributeValue{
		item("CLUSTER#prod", "prod", "arn:1", ""),
		item("CLUSTER#staging", "staging", "arn:2", ""),
	}}}

	got, err := listClusters(context.Background(), svc)
	if err != nil {
		t.Fatalf("listClusters: %v", err)
	}

	if len(got) != 2 || got[0] != "prod" || got[1] != "staging" {
		t.Errorf("got %v, want [prod staging]", got)
	}
}

// TestListClustersNoneIsNotAnError -- AskForCluster answers this with
// "no AppPack clusters are setup".
func TestListClustersNoneIsNotAnError(t *testing.T) {
	t.Parallel()

	svc := &fakeQuerier{out: &dynamodb.QueryOutput{}}

	got, err := listClusters(context.Background(), svc)
	if err != nil {
		t.Fatalf("listClusters: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestGetClusterItem(t *testing.T) {
	t.Parallel()

	svc := &fakeItemGetter{out: &dynamodb.GetItemOutput{
		Item: item("prod#DATABASE#main", "main", "arn:1", "postgres"),
	}}
	cluster, name := "prod", "main"

	got, err := getClusterItem(context.Background(), svc, &cluster, "DATABASE", &name)
	if err != nil {
		t.Fatalf("getClusterItem: %v", err)
	}

	if got.Name != "main" || got.DatabaseEngine != "postgres" || got.StackID != "arn:1" {
		t.Errorf("got %+v", *got)
	}

	key := stringAttr(t, svc.in.Key["secondary_id"])
	if key != "prod#DATABASE#main" {
		t.Errorf("secondary_id = %q, want prod#DATABASE#main", key)
	}
}

// A GetItem for a key that is not there really does come back with a nil
// Item, unlike Query's empty Items -- so this check is the real thing.
func TestGetClusterItemMissing(t *testing.T) {
	t.Parallel()

	svc := &fakeItemGetter{out: &dynamodb.GetItemOutput{}}
	cluster, name := "prod", "absent"

	_, err := getClusterItem(context.Background(), svc, &cluster, "DATABASE", &name)
	if err == nil {
		t.Fatal("got nil error for a key that is not there")
	}

	if err.Error() != "could not find CLUSTERS/prod#DATABASE#absent" {
		t.Errorf("error = %q", err.Error())
	}
}

func TestStackFromItem(t *testing.T) {
	t.Parallel()

	ddbSvc := &fakeItemGetter{out: &dynamodb.GetItemOutput{
		Item: item("CLUSTER#prod", "prod", "arn:cluster:1", ""),
	}}
	cfnSvc := &fakeStackDescriber{out: &cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackName: aws.String("apppack-cluster-prod")}},
	}}

	got, err := stackFromItem(context.Background(), ddbSvc, cfnSvc, "CLUSTER#prod")
	if err != nil {
		t.Fatalf("stackFromItem: %v", err)
	}

	if *got.StackName != "apppack-cluster-prod" {
		t.Errorf("StackName = %q", *got.StackName)
	}
}

func TestStackFromItemMissingItem(t *testing.T) {
	t.Parallel()

	ddbSvc := &fakeItemGetter{out: &dynamodb.GetItemOutput{}}

	_, err := stackFromItem(context.Background(), ddbSvc, &fakeStackDescriber{}, "CLUSTER#absent")
	if err == nil || err.Error() != "could not find CLUSTERS/CLUSTER#absent" {
		t.Errorf("error = %v", err)
	}
}

func TestStackFromItemNoStacks(t *testing.T) {
	t.Parallel()

	ddbSvc := &fakeItemGetter{out: &dynamodb.GetItemOutput{
		Item: item("CLUSTER#prod", "prod", "arn:cluster:1", ""),
	}}
	cfnSvc := &fakeStackDescriber{out: &cloudformation.DescribeStacksOutput{}}

	_, err := stackFromItem(context.Background(), ddbSvc, cfnSvc, "CLUSTER#prod")
	if err == nil || err.Error() != "no stacks found with ID arn:cluster:1" {
		t.Errorf("error = %v", err)
	}
}
