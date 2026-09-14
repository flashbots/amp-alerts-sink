package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/flashbots/amp-alerts-sink/publisher"
	"github.com/flashbots/amp-alerts-sink/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type metadataStubPublisher struct {
	name     string
	metadata publisher.Metadata
	err      error
	previous publisher.Results
	calls    *[]string
}

func (p *metadataStubPublisher) Name() string {
	return p.name
}

func (p *metadataStubPublisher) Publish(
	_ context.Context,
	_ string,
	_ *types.AlertmanagerAlert,
	previous publisher.Results,
) (publisher.Metadata, error) {
	p.previous = previous
	if p.calls != nil {
		*p.calls = append(*p.calls, p.name)
	}
	return p.metadata, p.err
}

func TestProcessorPassesPreviousPublisherResults(t *testing.T) {
	firstErr := errors.New("slack API unavailable")
	first := &metadataStubPublisher{
		name: publisher.NameSlack,
		metadata: publisher.Metadata{
			"channel_id": "C123",
		},
		err: firstErr,
	}
	second := &metadataStubPublisher{name: publisher.NameA2A}
	p := &Processor{
		ignoreRules: map[string]struct{}{},
		matchLabels: map[string]string{},
		log:         zap.NewNop(),
		publishers:  []publisher.Publisher{first, second},
	}

	err := p.processMessage(context.Background(), "test-source", &types.AlertmanagerMessage{
		Alerts: []types.AlertmanagerAlert{{
			Status:      "firing",
			StartsAt:    "2026-06-05T20:06:20Z",
			Labels:      map[string]string{"alertname": "SomeAlert"},
			Annotations: map[string]string{},
		}},
	})

	assert.ErrorIs(t, err, firstErr)
	require.Contains(t, second.previous, publisher.NameSlack)
	assert.Equal(t, publisher.ResultStatusFailed, second.previous[publisher.NameSlack].Status)
	assert.Equal(t, firstErr.Error(), second.previous[publisher.NameSlack].Error)
	assert.Equal(t, "C123", second.previous[publisher.NameSlack].Metadata["channel_id"])
}
