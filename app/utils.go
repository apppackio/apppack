package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/sirupsen/logrus"
)

func ddbItem(sess *session.Session, primaryID, secondaryID string) (*map[string]*dynamodb.AttributeValue, error) {
	ddbSvc := dynamodb.New(sess)

	logrus.WithFields(logrus.Fields{"primaryID": primaryID, "secondaryID": secondaryID}).Debug("DynamoDB GetItem")

	result, err := ddbSvc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("apppack"),
		Key: map[string]*dynamodb.AttributeValue{
			"primary_id": {
				S: aws.String(primaryID),
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
		return nil, fmt.Errorf("could not find DDB item %s %s", primaryID, secondaryID)
	}

	return &result.Item, nil
}

func SsmParameters(sess *session.Session, path string) ([]*ssm.Parameter, error) {
	ssmSvc := ssm.New(sess)

	var parameters []*ssm.Parameter

	input := ssm.GetParametersByPathInput{
		Path:           &path,
		WithDecryption: aws.Bool(true),
	}

	err := ssmSvc.GetParametersByPathPages(&input, func(resp *ssm.GetParametersByPathOutput, lastPage bool) bool {
		logrus.WithFields(logrus.Fields{"path": *input.Path}).Debug("loading parameter by path page")

		for _, parameter := range resp.Parameters {
			if parameter == nil {
				continue
			}

			parameters = append(parameters, parameter)
		}

		return !lastPage
	})
	if err != nil {
		return nil, err
	}

	return parameters, nil
}

func SsmParameter(sess *session.Session, name string) (*ssm.Parameter, error) {
	ssmSvc := ssm.New(sess)
	input := &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	}

	result, err := ssmSvc.GetParameter(input)
	if err != nil {
		return nil, err
	}

	return result.Parameter, nil
}

func S3FromURL(sess *session.Session, logURL string) (*strings.Builder, error) {
	s3Svc := s3.New(sess)
	parts := strings.Split(strings.TrimPrefix(logURL, "s3://"), "/")
	bucket := parts[0]
	object := strings.Join(parts[1:], "/")
	logrus.WithFields(logrus.Fields{"bucket": bucket, "key": object}).Debug("fetching object from S3")

	out, err := s3Svc.GetObject(&s3.GetObjectInput{
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
