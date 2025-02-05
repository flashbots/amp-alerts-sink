package publisher

import (
	"context"
	"reflect"
	"testing"

	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/flashbots/amp-alerts-sink/types"

	mock_db "github.com/flashbots/amp-alerts-sink/mock/db"
	mock_publisher "github.com/flashbots/amp-alerts-sink/mock/publisher"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	slack_api "github.com/slack-go/slack"
)

var (
	alertFiring = &types.AlertmanagerAlert{
		StartsAt: "2023-07-15T21:37:23.977957594Z",
		Status:   "firing",

		Annotations: map[string]string{
			"summary": "Notification test",
		},

		Labels: map[string]string{
			"alertname": "TestAlert",
			"instance":  "Grafana",
		},
	}

	alertResolved = &types.AlertmanagerAlert{
		StartsAt: "2023-07-15T21:37:23.977957594Z",
		Status:   "resolved",

		Annotations: map[string]string{
			"summary": "Notification test",
		},

		Labels: map[string]string{
			"alertname": "TestAlert",
			"instance":  "Grafana",
		},
	}
)

func setupSlackPublisher(t *testing.T, ctrl *gomock.Controller) (
	Publisher, *mock_db.MockDB, *mock_publisher.MockSlackApi,
) {
	db := mock_db.NewMockDB(ctrl)
	slackApi := mock_publisher.NewMockSlackApi(ctrl)

	slack, err := NewSlackChannel(&config.Slack{
		Token: "testToken",
		Channel: &config.SlackChannel{
			ID:   "testChannelID",
			Name: "testChannelName",
		},
	}, db)
	assert.NoError(t, err, "unexpected error while creating slack publisher")

	_slack, ok := slack.(*slackChannel)
	assert.True(t, ok, "unexpected underlying type for slack publisher: %s", reflect.TypeOf(slack))
	_slack.cli = slackApi

	return slack, db, slackApi
}

func TestSlackOpeningAlert(t *testing.T) {
	ctrl := gomock.NewController(t)
	p, db, slack := setupSlackPublisher(t, ctrl)
	ctx := context.Background()
	alert := alertFiring

	defer func() {
		err := p.Publish(ctx, "testSource", alert)
		assert.NoError(t, err)
	}()

	db.EXPECT().
		Get(ctx, "testSource/testChannelName/"+alert.MessageFingerprint()).
		Return("", nil)

	db.EXPECT().
		Lock(ctx, "testSource/testChannelName/"+alert.MessageFingerprint(), timeoutLock).
		Return(true, nil)

	db.EXPECT().
		Get(ctx, "testSource/testChannelName/"+alert.ThreadFingerprint()).
		Return("", nil) // no thread exists

	slack.EXPECT().
		PostMessage("testChannelName", gomock.Any()).
		DoAndReturn(func(channelID string, options ...slack_api.MsgOption) (string, string, error) {
			assert.Equal(t, "testChannelName", channelID)
			assert.Equal(t, 1, len(options))
			return "", "testMessageTS", nil
		})

	db.EXPECT().
		Set(ctx, "testSource/testChannelName/"+alert.MessageFingerprint(), timeoutThreadExpiry, "testMessageTS")

	db.EXPECT().
		Set(ctx, "testSource/testChannelName/"+alert.ThreadFingerprint(), timeoutThreadExpiry, "testMessageTS")

	slack.EXPECT().
		RemoveReaction("white_check_mark", gomock.Any()).
		DoAndReturn(func(_ string, item slack_api.ItemRef) error {
			assert.Equal(t, "testChannelID", item.Channel)
			assert.Equal(t, "testMessageTS", item.Timestamp)
			return nil
		})

	slack.EXPECT().
		AddReaction("rotating_light", gomock.Any()).
		DoAndReturn(func(_ string, item slack_api.ItemRef) error {
			assert.Equal(t, "testChannelID", item.Channel)
			assert.Equal(t, "testMessageTS", item.Timestamp)
			return nil
		})
}

func TestSlackFollowUpAlert(t *testing.T) {
	ctrl := gomock.NewController(t)
	p, db, slack := setupSlackPublisher(t, ctrl)
	ctx := context.Background()
	alert := alertFiring

	defer func() {
		err := p.Publish(ctx, "testSource", alert)
		assert.NoError(t, err)
	}()

	db.EXPECT().
		Get(ctx, "testSource/testChannelName/"+alert.MessageFingerprint()).
		Return("", nil)

	db.EXPECT().
		Lock(ctx, "testSource/testChannelName/"+alert.MessageFingerprint(), timeoutLock).
		Return(true, nil)

	db.EXPECT().
		Get(ctx, "testSource/testChannelName/"+alert.ThreadFingerprint()).
		Return("testThreadTS", nil) // thread exists

	slack.EXPECT().
		PostMessage("testChannelName", gomock.Any()).
		DoAndReturn(func(channelID string, options ...slack_api.MsgOption) (string, string, error) {
			assert.Equal(t, "testChannelName", channelID)
			assert.Equal(t, 2, len(options))
			return "", "testMessageTS", nil
		})

	db.EXPECT().
		Set(ctx, "testSource/testChannelName/"+alert.MessageFingerprint(), timeoutThreadExpiry, "testMessageTS")

	slack.EXPECT().
		RemoveReaction("white_check_mark", gomock.Any()).
		DoAndReturn(func(_ string, item slack_api.ItemRef) error {
			assert.Equal(t, "testChannelID", item.Channel)
			assert.Equal(t, "testThreadTS", item.Timestamp)
			return nil
		})

	slack.EXPECT().
		AddReaction("rotating_light", gomock.Any()).
		DoAndReturn(func(_ string, item slack_api.ItemRef) error {
			assert.Equal(t, "testChannelID", item.Channel)
			assert.Equal(t, "testThreadTS", item.Timestamp)
			return nil
		})
}

func TestSlackResolvingAlert(t *testing.T) {
	ctrl := gomock.NewController(t)
	p, db, slack := setupSlackPublisher(t, ctrl)
	ctx := context.Background()
	alert := alertResolved

	defer func() {
		err := p.Publish(ctx, "testSource", alert)
		assert.NoError(t, err)
	}()

	db.EXPECT().
		Get(ctx, "testSource/testChannelName/"+alert.MessageFingerprint()).
		Return("", nil)

	db.EXPECT().
		Lock(ctx, "testSource/testChannelName/"+alert.MessageFingerprint(), timeoutLock).
		Return(true, nil)

	db.EXPECT().
		Get(ctx, "testSource/testChannelName/"+alert.ThreadFingerprint()).
		Return("testThreadTS", nil) // thread exists

	slack.EXPECT().
		PostMessage("testChannelName", gomock.Any()).
		DoAndReturn(func(channelID string, options ...slack_api.MsgOption) (string, string, error) {
			assert.Equal(t, "testChannelName", channelID)
			assert.Equal(t, 2, len(options))
			return "", "testMessageTS", nil
		})

	db.EXPECT().
		Set(ctx, "testSource/testChannelName/"+alert.MessageFingerprint(), timeoutThreadExpiry, "testMessageTS")

	slack.EXPECT().
		RemoveReaction("rotating_light", gomock.Any()).
		DoAndReturn(func(_ string, item slack_api.ItemRef) error {
			assert.Equal(t, "testChannelID", item.Channel)
			assert.Equal(t, "testThreadTS", item.Timestamp)
			return nil
		})

	slack.EXPECT().
		AddReaction("white_check_mark", gomock.Any()).
		DoAndReturn(func(_ string, item slack_api.ItemRef) error {
			assert.Equal(t, "testChannelID", item.Channel)
			assert.Equal(t, "testThreadTS", item.Timestamp)
			return nil
		})
}

func TestSlackDuplicateAlert(t *testing.T) {
	ctrl := gomock.NewController(t)
	p, db, _ := setupSlackPublisher(t, ctrl)
	ctx := context.Background()
	alert := alertFiring

	defer func() {
		err := p.Publish(ctx, "testSource", alert)
		assert.NoError(t, err)
	}()

	db.EXPECT().
		Get(ctx, "testSource/testChannelName/"+alert.MessageFingerprint()).
		Return("testMessageTX", nil) // duplicate alert

}
