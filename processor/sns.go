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
		raw := []byte(r.SNS.Message)
		m := &types.AlertmanagerMessage{}

		err := json.Unmarshal(raw, m)
		if err != nil && err.Error() == "invalid character '\\'' in string escape code" {
			raw = sanitiseJsEscapedApostrophes(raw)
			err = json.Unmarshal(raw, m)
		}
		if err != nil {
			l.Error("Error un-marshalling message",
				zap.String("message", strings.ReplaceAll(r.SNS.Message, "\n", " ")),
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

// sanitiseJsEscapedApostrophes replaces escaped apostrophes (\') for just
// apostrophes (') in the byte slice so that we could try to parse strings
// that were processes by `js` function of AMP's alertmanager template engine
func sanitiseJsEscapedApostrophes(input []byte) []byte {
	res := make([]byte, 0, len(input))

	var inString, sawEscape bool

	for _, char := range input {
		if !inString {
			if char == '"' {
				inString = true
			}
			res = append(res, char)
			continue
		}

		// in string

		if sawEscape {
			if char == '\'' {
				res = append(res, '\'') // substitute \' with ' inside strings
			} else {
				res = append(res, '\\', char)
			}
			sawEscape = false
			continue
		}

		switch char {
		case '\\':
			sawEscape = true
		case '"':
			inString = false
			res = append(res, char)
		default:
			res = append(res, char)
		}
	}

	return res
}
