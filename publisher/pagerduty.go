package publisher

// TODO: switch to 'go tool' once 1.24 is released
//go:generate mockgen -package mock_publisher -destination ../mock/publisher/pagerduty.go -source pagerduty.go  -mock_names pagerDutyClient=Mock_pagerDutyClient pagerDutyClient

import (
	"context"
	"fmt"
	"maps"
	"slices"
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
			l.Error("Failed to publish alert to pagerduty", zap.Error(err))

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

	event := &pagerduty.V2Event{
		RoutingKey: p.integrationKey,
		DedupKey:   alert.IncidentDedupKey(),
		Payload: &pagerduty.V2Payload{
			Timestamp: alert.StartsAt,
			Class:     alert.Labels["alertname"],
		},
	}

	event.Action = "trigger"
	if alert.Status == "resolved" {
		event.Action = "resolve"
	}

	addLink := func(href, text string) {
		if href == "" {
			return
		}
		event.Links = append(event.Links, map[string]string{"href": href, "text": text})
	}
	addLink(alert.Annotations["runbook_url"], "📕 Runbook")
	addLink(alert.GeneratorURL, "📈 Expr")
	addLink(alert.SilenceURL, "🔕 Silence")

	// Use SNS topic ARN, unless the "source" label is set
	event.Client = source
	if src := alert.Labels["source"]; src != "" {
		event.Client = src
	}
	event.ClientURL = alert.GeneratorURL

	event.Payload.Summary = alert.Labels["alertname"]
	if summary := alert.Annotations["summary"]; summary != "" {
		event.Payload.Summary += ": " + summary
	}

	// "instance" is always set by scraper, but "instance_name" is optional
	// Source in payload should be "unique location of the affected system"
	// as per PagerDuty docs
	event.Payload.Source = alert.Labels["instance"]
	if src := alert.Labels["instance_name"]; src != "" {
		event.Payload.Source = src
	}

	severity := alert.Labels["severity"]
	if !slices.Contains([]string{"critical", "warning", "error", "info"}, severity) {
		// Default severity to critical, if it's not one of the PD valid values.
		// On invalid severity, PagerDuty will not create an incident.
		severity = "critical"
	}
	event.Payload.Severity = severity

	details := maps.Clone(alert.Labels)
	delete(details, "severity")
	delete(details, "alertname")
	maps.Copy(details, alert.Annotations)
	delete(details, "summary")
	event.Payload.Details = details

	l.Info(
		"Publishing alert to pagerduty",
		zap.Any("alert", alert),
		zap.Any("event", event),
	)
	resp, err := p.client.ManageEventWithContext(ctx, event)
	if err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return fmt.Errorf("pagerduty: %v", resp.Errors)
	}
	l.Info("Successfully published to pagerduty")
	return nil
}
