package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ValidateEventbridgeCron validates a cron schedule rule
func (a *AWS) ValidateEventbridgeCron(rule string) error {
	eventSvc := eventbridge.NewFromConfig(a.cfg)
	ruleName := "apppack-validate-" + uuid.New().String()
	_, err := eventSvc.PutRule(context.Background(), &eventbridge.PutRuleInput{
		Name:               aws.String(ruleName),
		ScheduleExpression: aws.String(fmt.Sprintf("cron(%s)", rule)),
		State:              types.RuleStateDisabled,
	})

	if err == nil {
		_, err2 := eventSvc.DeleteRule(context.Background(), &eventbridge.DeleteRuleInput{
			Name: aws.String(ruleName),
		})
		if err2 != nil {
			logrus.WithError(err2).Debug("failed to delete temporary validation rule")
		}
	}

	return err
}
