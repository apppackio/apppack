package auth

import "fmt"

type Role interface {
	GetRoleARN() string
}

type AppRole struct {
	RoleARN   string `json:"role_arn" dynamodbav:"role_arn"`
	AccountID string `json:"account_id"`
	AppName   string `json:"name" dynamodbav:"secondary_id"`
	Region    string `json:"region" dynamodbav:"region"`
	Pipeline  bool   `json:"pipeline" dynamodbav:"pipeline"`
}

func (a *AppRole) GetRoleARN() string {
	return a.RoleARN
}

type AdminRole struct {
	RoleARN      string `json:"role_arn" dynamodbav:"role_arn"`
	AccountID    string `json:"account_id" dynamodbav:"secondary_id"`
	AccountAlias string `json:"alias"`
	Region       string `json:"region" dynamodbav:"region"`
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
