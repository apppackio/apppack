package auth

import "fmt"

type Role interface {
	GetRoleARN() string
}

type AppRole struct {
	RoleARN   string `dynamodbav:"role_arn"      json:"role_arn"`
	AccountID string `json:"account_id"`
	AppName   string `dynamodbav:"secondary_id"  json:"name"`
	Region    string `dynamodbav:"region"        json:"region"`
	Pipeline  bool   `dynamodbav:"pipeline"      json:"pipeline"`
}

func (a *AppRole) GetRoleARN() string {
	return a.RoleARN
}

type AdminRole struct {
	RoleARN      string `dynamodbav:"role_arn"      json:"role_arn"`
	AccountID    string `dynamodbav:"secondary_id"  json:"account_id"`
	AccountAlias string `json:"alias"`
	Region       string `dynamodbav:"region"        json:"region"`
}

func (a *AdminRole) GetRoleARN() string {
	return a.RoleARN
}

func (a *AdminRole) GetAccountName() string {
	if a.AccountAlias != "" {
		return fmt.Sprintf("%s (%s)", a.AccountID, a.AccountAlias)
	}
	return a.AccountID
}
