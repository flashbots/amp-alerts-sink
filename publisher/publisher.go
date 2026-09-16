package publisher

import (
	"context"
	"errors"
	"time"

	"github.com/flashbots/amp-alerts-sink/types"
)

type Publisher interface {
	Name() string

	Publish(
		ctx context.Context,
		source string,
		alert *types.AlertmanagerAlert,
		previous PublishResults,
	) (PublishMetadata, PublisherError)
}

type PublisherError interface {
	error
	IsReportable() bool
}

type PublishMetadata map[string]any

type Result struct {
	Status   string          `json:"status"`
	Metadata PublishMetadata `json:"metadata,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type PublishResults map[string]Result

const (
	ResultStatusAlreadyLocked = "already-locked"
	ResultStatusFailed        = "failed"
	ResultStatusSucceeded     = "succeeded"

	NameA2A       = "a2a"
	NamePagerDuty = "pagerduty"
	NameSlack     = "slack"
	NameWebhook   = "webhook"
)

const (
	timeoutA2AExpiry     = 30 * 24 * time.Hour
	timeoutLock          = time.Second
	timeoutThreadExpiry  = 30 * 24 * time.Hour
	timeoutWebhookExpiry = 30 * 24 * time.Hour
)

var (
	ErrAlreadyPublishing = errors.New("the message is being published, let's retry later")
)
