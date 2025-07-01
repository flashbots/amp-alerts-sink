package processor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/flashbots/amp-alerts-sink/types"

	"go.uber.org/zap"
)

func (p *Processor) ProcessSnsEvent(ctx context.Context, event events.SNSEvent) error {
	l := p.log
	defer l.Sync() //nolint:errcheck

	errs := []error{}
	for _, r := range event.Records {
		m := &types.AlertmanagerMessage{}
		if err := json.Unmarshal([]byte(r.SNS.Message), m); err != nil {
			l.Error("Error un-marshalling message",
				zap.String("message", strings.Replace(r.SNS.Message, "\n", " ", -1)),
				zap.Error(err),
			)
			errs = append(errs, err)
			continue
		}
		if err := p.processMessage(ctx, r.SNS.TopicArn, m); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		alert := &types.AlertmanagerMessage{
			Alerts: []types.AlertmanagerAlert{{
				Status:   "firing",
				StartsAt: time.Now().UTC().Format(timeFormatPrometheus),
				Labels: map[string]string{
					"alertname": "AMPAlertsSinkParseError",
					"severity":  "critical",
				},
				Annotations: map[string]string{
					"summary": "Failed to parse SNS messages",
					"description": "amp-alerts-sink failed to process some alerts. " +
						"Check Lambda logs for more details.",
				},
			}},
		}
		if err := p.processMessage(ctx, "amp-alerts-sink", alert); err != nil {
			l.Error("Failed to send parse error alert", zap.Error(err))
		}
	}
	return errors.Join(errs...)
}
