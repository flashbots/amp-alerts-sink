package processor

import (
	"context"
	"errors"
	"time"

	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/flashbots/amp-alerts-sink/db"
	"github.com/flashbots/amp-alerts-sink/logutils"
	"github.com/flashbots/amp-alerts-sink/publisher"
	"github.com/flashbots/amp-alerts-sink/types"
	"go.uber.org/zap"
)

const (
	timeFormatGrafana    = "2006-01-02T15:04:05Z07:00"
	timeFormatPrometheus = "2006-01-02 15:04:05.999999999 -0700 MST"
)

var (
	ErrPublisherUndefined = errors.New("no publishers defined")
)

type Processor struct {
	ignoreRules map[string]struct{}
	matchLabels map[string]string
	log         *zap.Logger
	publishers  []publisher.Publisher
}

func New(cfg *config.Config) (*Processor, error) {
	db, err := db.New(cfg.DynamoDB)
	if err != nil {
		return nil, err
	}

	publishers := make([]publisher.Publisher, 0)
	if cfg.Slack.Enabled() {
		slack, err := publisher.NewSlackChannel(
			cfg.Slack,
			db.WithNamespace("slack/"+cfg.Slack.Channel.Name),
		)
		if err != nil {
			return nil, err
		}
		publishers = append(publishers, slack)
	}

	if len(publishers) == 0 {
		return nil, ErrPublisherUndefined
	}

	ignoreRules := make(map[string]struct{}, len(cfg.Processor.IgnoreRules))
	for _, r := range cfg.Processor.IgnoreRules {
		ignoreRules[r] = struct{}{}
	}

	return &Processor{
		ignoreRules: ignoreRules,
		matchLabels: cfg.Processor.MatchLabels,
		log:         zap.L(),
		publishers:  publishers,
	}, nil
}

func (p *Processor) processMessage(
	ctx context.Context,
	source string,
	message *types.AlertmanagerMessage,
) error {
	errs := []error{}
	for _, alert := range message.Alerts {
		// merge common labels into alert's labels
		for k, v := range message.CommonLabels {
			if _, present := alert.Labels[k]; !present {
				alert.Labels[k] = v
			}
		}

		// merge common annotations into alert's annotations
		for k, v := range message.CommonAnnotations {
			if _, present := alert.Annotations[k]; !present {
				alert.Annotations[k] = v
			}
		}

		// create alert-specific logger
		l := logutils.LoggerFromContext(ctx).With(
			zap.String("alert_fingerprint", alert.MessageFingerprint()),
			zap.String("alert_labels_fingerprint", alert.ThreadFingerprint()),
		)
		ctx = logutils.ContextWithLogger(ctx, l)

		// skip ignored alerts
		if _, ignore := p.ignoreRules[alert.Labels["alertname"]]; ignore {
			l.Info("Skipped the alert according to ignore-rules configuration",
				zap.Any("alert", alert),
			)
			return nil
		}

		// skip un-matched alerts
		if len(p.matchLabels) > 0 {
			for label, value := range p.matchLabels {
				if alert.Labels[label] != value {
					l.Info("Skipped the alert due to label mismatch",
						zap.Any("alert", alert),
						zap.String("label", label),
						zap.String("expected", value),
					)
					return nil
				}
			}
		}

		// normalise alert's timestamp
		_timestamp, err := time.Parse(timeFormatGrafana, alert.StartsAt)
		if err != nil {
			_timestamp, err = time.Parse(timeFormatPrometheus, alert.StartsAt)
		}
		if err == nil {
			alert.StartsAt = _timestamp.Format(timeFormatGrafana)
		}

		// publish
		for _, pub := range p.publishers {
			if err := pub.Publish(ctx, source, &alert); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) != 0 {
		return errors.Join(errs...)
	}
	return nil
}
