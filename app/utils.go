package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/sirupsen/logrus"
)

func ddbItem(cfg aws.Config, primaryID, secondaryID string) (*map[string]types.AttributeValue, error) {
	ddbSvc := dynamodb.NewFromConfig(cfg)

	logrus.WithFields(logrus.Fields{"primaryID": primaryID, "secondaryID": secondaryID}).Debug("DynamoDB GetItem")

	tableName := "apppack"
	result, err := ddbSvc.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: &tableName,
		Key: map[string]types.AttributeValue{
			"primary_id": &types.AttributeValueMemberS{
				Value: primaryID,
			},
			"secondary_id": &types.AttributeValueMemberS{
				Value: secondaryID,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, fmt.Errorf("could not find DDB item %s %s", primaryID, secondaryID)
	}

	return &result.Item, nil
}

func SsmParameters(cfg aws.Config, path string) ([]ssmtypes.Parameter, error) {
	ssmSvc := ssm.NewFromConfig(cfg)

	var parameters []ssmtypes.Parameter

	withDecryption := true
	paginator := ssm.NewGetParametersByPathPaginator(ssmSvc, &ssm.GetParametersByPathInput{
		Path:           &path,
		WithDecryption: &withDecryption,
	})

	for paginator.HasMorePages() {
		resp, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, err
		}

		logrus.WithFields(logrus.Fields{"path": path}).Debug("loading parameter by path page")
		parameters = append(parameters, resp.Parameters...)
	}

	return parameters, nil
}

func SsmParameter(cfg aws.Config, name string) (*ssmtypes.Parameter, error) {
	ssmSvc := ssm.NewFromConfig(cfg)
	withDecryption := true
	input := &ssm.GetParameterInput{
		Name:           &name,
		WithDecryption: &withDecryption,
	}

	result, err := ssmSvc.GetParameter(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return result.Parameter, nil
}

func S3FromURL(cfg aws.Config, logURL string) (*strings.Builder, error) {
	s3Svc := s3.NewFromConfig(cfg)
	parts := strings.Split(strings.TrimPrefix(logURL, "s3://"), "/")
	bucket := parts[0]
	object := strings.Join(parts[1:], "/")
	logrus.WithFields(logrus.Fields{"bucket": bucket, "key": object}).Debug("fetching object from S3")

	out, err := s3Svc.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &object,
	})
	if err != nil {
		return nil, err
	}

	buf := new(strings.Builder)

	_, err = io.Copy(buf, out.Body)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

var JSONIndent = "  "

func toJSON(v interface{}) (*bytes.Buffer, error) {
	j, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer([]byte{})

	if err = json.Indent(buf, j, "", JSONIndent); err != nil {
		return nil, err
	}

	return buf, nil
}
