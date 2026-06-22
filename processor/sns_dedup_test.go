package processor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/flashbots/amp-alerts-sink/publisher"
	"github.com/flashbots/amp-alerts-sink/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// stubPublisher records every published alert and returns a fixed error.
type stubPublisher struct {
	err   error
	calls []types.AlertmanagerAlert
}

func (s *stubPublisher) Publish(_ context.Context, _ string, alert *types.AlertmanagerAlert) error {
	s.calls = append(s.calls, *alert)
	return s.err
}

func newTestProcessor(pub publisher.Publisher) *Processor {
	return &Processor{
		ignoreRules: map[string]struct{}{},
		matchLabels: map[string]string{},
		log:         zap.NewNop(),
		publishers:  []publisher.Publisher{pub},
	}
}

func snsEventWithAlert(t *testing.T) events.SNSEvent {
	t.Helper()
	raw, err := json.Marshal(types.AlertmanagerMessage{
		Status: "firing",
		Alerts: []types.AlertmanagerAlert{{
			Status:   "firing",
			StartsAt: "2026-06-05T20:06:20Z",
			Labels:   map[string]string{"alertname": "SomeAlert", "severity": "warning"},
		}},
	})
	require.NoError(t, err)
	return events.SNSEvent{Records: []events.SNSEventRecord{{
		SNS: events.SNSEntity{TopicArn: "test-topic", Message: string(raw)},
	}}}
}

// When an HA peer wins the dedup lock race, the publisher returns
// ErrAlreadyLocked. That's a normal, harmless outcome: the invocation should
// still succeed and no AMPAlertsSinkParseError alert should go out.
func TestProcessSnsEvent_AlreadyLockedSucceeds(t *testing.T) {
	pub := &stubPublisher{err: publisher.ErrAlreadyLocked}
	p := newTestProcessor(pub)

	err := p.ProcessSnsEvent(context.Background(), snsEventWithAlert(t))

	assert.NoError(t, err)
	require.Len(t, pub.calls, 1, "only the original alert; no synthetic parse-error alert")
	assert.Equal(t, "SomeAlert", pub.calls[0].Labels["alertname"])
}

// A genuine publish error should still fail the invocation and send out the
// AMPAlertsSinkParseError alert.
func TestProcessSnsEvent_RealErrorEmitsParseErrorAlert(t *testing.T) {
	pub := &stubPublisher{err: errors.New("boom")}
	p := newTestProcessor(pub)

	err := p.ProcessSnsEvent(context.Background(), snsEventWithAlert(t))

	assert.Error(t, err)
	require.Len(t, pub.calls, 2, "original alert plus synthetic parse-error alert")
	assert.Equal(t, "AMPAlertsSinkParseError", pub.calls[1].Labels["alertname"])
}
