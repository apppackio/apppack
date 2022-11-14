package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eventbridge"
	"github.com/google/uuid"
)

// ValidateEventbridgeCron validates a cron schedule rule
func (a *AWS) ValidateEventbridgeCron(rule string) error {
	eventSvc := eventbridge.New(a.session)
	ruleName := fmt.Sprintf("apppack-validate-%s", uuid.New().String())
	_, err := eventSvc.PutRule(&eventbridge.PutRuleInput{
		Name:               &ruleName,
		ScheduleExpression: aws.String(fmt.Sprintf("cron(%s)", rule)),
		State:              aws.String("DISABLED"),
	})
	if err == nil {
		eventSvc.DeleteRule(&eventbridge.DeleteRuleInput{
			Name: &ruleName,
		})
	}
	return err
}
