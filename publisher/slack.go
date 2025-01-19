package publisher

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/flashbots/amp-alerts-sink/db"
	"github.com/flashbots/amp-alerts-sink/logutils"
	"github.com/flashbots/amp-alerts-sink/types"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type slackChannel struct {
	channelID   string
	channelName string

	cli SlackApi
	db  db.DB
}

type SlackApi interface {
	AddReaction(name string, item slack.ItemRef) error
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
	RemoveReaction(name string, item slack.ItemRef) error
}

var (
	ErrAlreadyLocked = errors.New("the message is already locked, let's retry later")
)

func NewSlackChannel(cfg *config.Slack, db db.DB) (Publisher, error) {
	return &slackChannel{
		channelName: cfg.Channel.Name,
		channelID:   cfg.Channel.ID,

		cli: slack.New(cfg.Token),
		db:  db,
	}, nil
}

func (s *slackChannel) Publish(
	ctx context.Context,
	source string,
	alert *types.AlertmanagerAlert,
) (err error) {
	l := logutils.LoggerFromContext(ctx)

	dbKeyThreadTS := source + "/" + s.channelName + "/" + alert.ThreadFingerprint()
	dbKeyMessageTS := source + "/" + s.channelName + "/" + alert.MessageFingerprint()

	var messageTS, threadTS string

	// whatever the issues with DB we will try to publish to slack at least once
	alreadyPublished := false
	message := s.newMessage(alert)
	defer func() {
		if !alreadyPublished {
			_, err2 := s.publishMessage(ctx, message, threadTS)
			if err2 == nil {
				l.Warn("Emergency-published the alert to slack",
					zap.Any("alert", alert),
				)
			}
			err = errors.Join(err, err2)
		}
	}()

	// fetch timestamps from the db (if present)
	messageTS, err = s.db.Get(ctx, dbKeyMessageTS)
	if err != nil {
		return err
	}

	// check if this message was already published
	if len(messageTS) > 0 {
		alreadyPublished = true
		return nil
	}

	// try to lock the db
	didLock, err := s.db.Lock(ctx, dbKeyMessageTS, timeoutLock)
	if !didLock && err == nil {
		// another grafana's HA instance is about to publish
		alreadyPublished = true
		return ErrAlreadyLocked
	}

	// check if this is a follow-up message
	threadTS, err = s.db.Get(ctx, dbKeyThreadTS)
	if err != nil {
		return err
	}

	// send message to slack
	messageTS, err = s.publishMessage(ctx, message, threadTS)
	if err != nil {
		return err
	}
	alreadyPublished = true
	l.Info("Published alert to slack",
		zap.Any("alert", alert),
	)

	// make sure we don't re-publish it from another HA instance
	_ = s.db.Set(ctx, dbKeyMessageTS, timeoutThreadExpiry, messageTS)

	if len(threadTS) == 0 {
		// set thread's timestamp to be the same as the timestamp of its first message
		threadTS = messageTS
		_ = s.db.Set(ctx, dbKeyThreadTS, timeoutThreadExpiry, threadTS)
	}

	// update reaction emoji on the thread-starting message
	if len(threadTS) > 0 {
		s.updateReaction(ctx, alert, threadTS)
	}

	return nil
}

func (s *slackChannel) newMessage(
	alert *types.AlertmanagerAlert,
) slack.Attachment {
	msg := slack.Attachment{}

	if alert.Status == "firing" {
		switch alert.Labels["severity"] {
		case "critical":
			msg.Color = "danger"
		case "warning":
			msg.Color = "warning"
		default:
			msg.Color = "good"
		}
	} else {
		msg.Color = "good"
	}

	msg.Title = fmt.Sprintf("%s: %s",
		strings.ToUpper(alert.Status),
		alert.Labels["alertname"],
	)

	if alertSeverity, ok := alert.Labels["severity"]; ok {
		msg.Text += fmt.Sprintf("Severity: `%s`\n", alertSeverity)
	}
	if alertSummary, ok := alert.Annotations["summary"]; ok {
		msg.Text += fmt.Sprintf("Summary: `%s`\n", alertSummary)
	}
	if alertDescription, ok := alert.Annotations["description"]; ok {
		msg.Text += fmt.Sprintf("\n%s\n\n", alertDescription)
	}
	if alertMessage, ok := alert.Annotations["message"]; ok {
		msg.Text += fmt.Sprintf("\n%s\n\n", alertMessage)
	}
	if len(alert.StartsAt) > 0 {
		msg.Text += fmt.Sprintf("Started at: `%s`\n", alert.StartsAt)
	}
	if awsAccount, ok := alert.Labels["aws_account"]; ok {
		msg.Text += fmt.Sprintf("AWS account: `%s`\n", awsAccount)
	}
	if cluster, ok := alert.Labels["cluster"]; ok {
		msg.Text += fmt.Sprintf("Kubernetes cluster: `%s`\n", cluster)
	}
	if namespace, ok := alert.Labels["namespace"]; ok {
		msg.Text += fmt.Sprintf("Kubernetes namespace: `%s`\n", namespace)
	}

	return msg
}

func (s *slackChannel) publishMessage(
	ctx context.Context,
	message slack.Attachment,
	threadTS string,
) (string, error) {
	l := logutils.LoggerFromContext(ctx)

	if len(threadTS) > 0 {
		if floatThreadTS, err := strconv.ParseFloat(threadTS, 64); err == nil {
			sec, dec := math.Modf(floatThreadTS)
			timeSlackThreadTS := time.Unix(int64(sec), int64(dec*(1e9)))
			message.Footer = fmt.Sprintf("(follow-up to the alert published at %s)",
				timeSlackThreadTS.Format("2006-01-02T15:04:05Z07:00"),
			)
		} else {
			message.Footer = "(follow-up)"
		}
	}

	opts := []slack.MsgOption{
		slack.MsgOptionAttachments(message),
	}
	if len(threadTS) > 0 {
		opts = append(opts,
			slack.MsgOptionTS(threadTS),
		)
	}

	_, messageTS, err := s.cli.PostMessage(s.channelName, opts...)
	if err != nil {
		l.Error("Error publishing message to slack",
			zap.Error(err),
			zap.String("slack_channel", s.channelName),
			zap.String("slack_message_ts", messageTS),
			zap.String("slack_thread_ts", threadTS),
		)
		return "", err
	}

	return messageTS, nil
}

func (s *slackChannel) updateReaction(
	ctx context.Context,
	alert *types.AlertmanagerAlert,
	threadTS string,
) {
	l := logutils.LoggerFromContext(ctx)

	var ra, rr string
	switch alert.Status {
	case "firing":
		ra = "rotating_light"
		rr = "white_check_mark"
	case "resolved":
		ra = "white_check_mark"
		rr = "rotating_light"
	default:
		return
	}

	if err := func() error {
		err := s.cli.AddReaction(ra, slack.ItemRef{
			Channel:   s.channelID,
			Timestamp: threadTS,
		})
		if err == nil {
			return nil
		}
		slackErr, isSlackErr := err.(slack.SlackErrorResponse)
		if !isSlackErr {
			return err
		}
		if slackErr.Err == "already_reacted" {
			return nil
		}
		return err
	}(); err != nil {
		l.Error("Error adding reaction to slack",
			zap.Error(err),
			zap.String("slack_channel", s.channelName),
			zap.String("slack_reaction", ra),
			zap.String("slack_thread_ts", threadTS),
		)
	}

	if err := func() error {
		err := s.cli.RemoveReaction(rr, slack.ItemRef{
			Channel:   s.channelID,
			Timestamp: threadTS,
		})
		if err == nil {
			return nil
		}
		slackErr, isSlackErr := err.(slack.SlackErrorResponse)
		if !isSlackErr {
			return err
		}
		if slackErr.Err == "no_reaction" {
			return nil
		}
		return err
	}(); err != nil {
		l.Error("Error removing reaction from slack",
			zap.Error(err),
			zap.String("slack_channel", s.channelName),
			zap.String("slack_reaction", rr),
			zap.String("slack_thread_ts", threadTS),
		)
	}
}
