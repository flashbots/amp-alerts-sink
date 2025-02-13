package publisher

import (
	"context"
	"testing"

	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/flashbots/amp-alerts-sink/types"
	"github.com/stretchr/testify/assert"

	mock_publisher "github.com/flashbots/amp-alerts-sink/mock/publisher"

	"github.com/PagerDuty/go-pagerduty"
	"go.uber.org/mock/gomock"
)

func setupPagerDutyPublisher(t *testing.T) (Publisher, *mock_publisher.Mock_pagerDutyClient) {
	ctrl := gomock.NewController(t)

	pdMock := mock_publisher.NewMock_pagerDutyClient(ctrl)

	pd := NewPagerDuty(&config.PagerDuty{
		IntegrationKey: "theKey",
	})

	_pd := pd.(pagerDuty)
	_pd.client = pdMock

	return _pd, pdMock
}

func TestPagerDutyAlertCreation(t *testing.T) {
	p, pdMock := setupPagerDutyPublisher(t)
	ctx := context.Background()
	alert := alertFiring

	pdMock.EXPECT().
		ManageEventWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, event *pagerduty.V2Event) (*pagerduty.V2EventResponse, error) {
			assert.Equal(t, "theKey", event.RoutingKey)
			assert.Equal(t, "trigger", event.Action)
			assert.Equal(t, alert.IncidentDedupKey(), event.DedupKey)
			assert.Equal(t, "TestAlert: Notification test", event.Payload.Summary)
			assert.Equal(t, "critical", event.Payload.Severity)
			return &pagerduty.V2EventResponse{}, nil
		})

	err := p.Publish(ctx, "testSource", alert)
	assert.NoError(t, err)
}

func TestPagerDutyDuplicateAlert(t *testing.T) {
	p, pdMock := setupPagerDutyPublisher(t)
	ctx := context.Background()
	alert := alertFiring

	pdMock.EXPECT().
		ManageEventWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, event *pagerduty.V2Event) (*pagerduty.V2EventResponse, error) {
			assert.Equal(t, alert.IncidentDedupKey(), event.DedupKey)
			return &pagerduty.V2EventResponse{}, nil
		}).Times(2)

	err := p.Publish(ctx, "testSource", alert)
	assert.NoError(t, err)

	// Change the alert annotations, but keep the same dedup key
	alert.Annotations["foo"] = "bar"
	err = p.Publish(ctx, "testSource", alert)
	assert.NoError(t, err)
}

func TestPagerDutyDifferentDedupKey(t *testing.T) {
	p, pdMock := setupPagerDutyPublisher(t)
	ctx := context.Background()
	alert1 := alertFiring
	alert2 := &types.AlertmanagerAlert{
		StartsAt: alert1.StartsAt,
		Status:   "firing",

		Annotations: map[string]string{
			"summary": "Another notification test",
		},

		Labels: map[string]string{
			"alertname": "AnotherTestAlert",
			"severity":  "warning",
		},
	}

	pdMock.EXPECT().
		ManageEventWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, event *pagerduty.V2Event) (*pagerduty.V2EventResponse, error) {
			if event.DedupKey == alert1.IncidentDedupKey() {
				assert.Equal(t, "TestAlert: Notification test", event.Payload.Summary)
				assert.Equal(t, "critical", event.Payload.Severity)
			} else if event.DedupKey == alert2.IncidentDedupKey() {
				assert.Equal(t, "AnotherTestAlert: Another notification test", event.Payload.Summary)
				assert.Equal(t, "warning", event.Payload.Severity)
			}
			return &pagerduty.V2EventResponse{}, nil
		}).Times(2)

	err := p.Publish(ctx, "testSource", alert1)
	assert.NoError(t, err)

	err = p.Publish(ctx, "testSource", alert2)
	assert.NoError(t, err)
}

func TestPagerDutyResolveAlert(t *testing.T) {
	p, pdMock := setupPagerDutyPublisher(t)
	ctx := context.Background()
	alertFiring := alertFiring
	alertResolved := alertResolved

	// Expect the alert to be fired first
	pdMock.EXPECT().
		ManageEventWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, event *pagerduty.V2Event) (*pagerduty.V2EventResponse, error) {
			assert.Equal(t, alertFiring.IncidentDedupKey(), event.DedupKey)
			assert.Equal(t, "trigger", event.Action)
			return &pagerduty.V2EventResponse{}, nil
		})

	// Fire the alert
	err := p.Publish(ctx, "testSource", alertFiring)
	assert.NoError(t, err)

	// Expect the alert to be resolved
	pdMock.EXPECT().
		ManageEventWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, event *pagerduty.V2Event) (*pagerduty.V2EventResponse, error) {
			assert.Equal(t, alertResolved.IncidentDedupKey(), event.DedupKey)
			assert.Equal(t, "resolve", event.Action)
			return &pagerduty.V2EventResponse{}, nil
		})

	// Resolve the alert
	err = p.Publish(ctx, "testSource", alertResolved)
	assert.NoError(t, err)
}
