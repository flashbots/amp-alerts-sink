package publisher

// TODO: switch to 'go tool' once 1.24 is released
//go:generate mockgen -package mock_publisher -destination ../mock/publisher/pagerduty.go -source pagerduty.go  -mock_names pagerDutyClient=Mock_pagerDutyClient pagerDutyClient

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/flashbots/amp-alerts-sink/logutils"
	"github.com/flashbots/amp-alerts-sink/types"

	"github.com/PagerDuty/go-pagerduty"
	"go.uber.org/zap"
)

func NewPagerDuty(cfg *config.PagerDuty) Publisher {
	return pagerDuty{
		integrationKey: cfg.IntegrationKey,
		client:         pagerduty.NewClient("auth-token-unused"),
	}
}

type pagerDuty struct {
	integrationKey string
	client         pagerDutyClient
}

type pagerDutyClient interface {
	ManageEventWithContext(context.Context, *pagerduty.V2Event) (*pagerduty.V2EventResponse, error)
}

func (p pagerDuty) Publish(
	ctx context.Context,
	source string,
	alert *types.AlertmanagerAlert,
) (err error) {
	l := logutils.LoggerFromContext(ctx)

	defer func() {
		if err != nil {
			l.Error("Failed to publish alert to pagerduty",
				zap.Error(err),
				zap.Any("alert", alert),
			)

			// If we fail to publish the alert, we should publish another alert
			// to notify that something is wrong.
			// Not including dedup_key so to spawn a new incident.

			errStr := err.Error()
			if len(errStr) > 1024 {
				errStr = errStr[:1024] // so that we don't accidentally exceed the size limit
			}
			errEvent := &pagerduty.V2Event{
				RoutingKey: p.integrationKey,
				Action:     "trigger",
				Payload: &pagerduty.V2Payload{
					Summary:  "Failed to post alert to pagerduty",
					Source:   "amp-alerts-sink",
					Severity: "critical",
					Details: map[string]string{
						"err":  errStr,
						"text": "Check AWS lambda logs for more details",
					},
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, eerr := p.client.ManageEventWithContext(ctx, errEvent)
			if eerr != nil {
				l.Error("Failed to publish error alert to pagerduty",
					zap.Error(eerr),
				)
			} else if len(resp.Errors) > 0 {
				l.Error("Failed to publish error alert to pagerduty",
					zap.Any("errors", resp.Errors),
				)
			} else {
				l.Info("Published error alert to pagerduty")
			}
		}
	}()

	action := "trigger"
	if alert.Status == "resolved" {
		action = "resolve"
	}

	summary := alert.Labels["alertname"]
	if alert.Annotations["summary"] != "" {
		summary += fmt.Sprint(": ", alert.Annotations["summary"])
	}

	event := &pagerduty.V2Event{
		RoutingKey: p.integrationKey,
		Action:     action,
		DedupKey:   alert.IncidentDedupKey(),
		ClientURL:  alert.Annotations["generatorURL"],
		Payload: &pagerduty.V2Payload{
			Source:    source,
			Timestamp: alert.StartsAt,
			Summary:   summary,
			Severity:  alert.Labels["severity"],
			Class:     alert.Labels["alertname"],
		},
	}

	details := maps.Clone(alert.Labels)
	delete(details, "severity")
	delete(details, "alertname")
	maps.Copy(details, alert.Annotations)
	delete(details, "summary")
	delete(details, "generatorURL")
	event.Payload.Details = details

	resp, err := p.client.ManageEventWithContext(ctx, event)
	if err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return fmt.Errorf("pagerduty: %v", resp.Errors)
	}
	l.Info("Published alert to pagerduty",
		zap.Any("alert", alert),
	)
	return nil
}
