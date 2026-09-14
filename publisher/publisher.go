package publisher

import (
	"context"
	"time"

	"github.com/flashbots/amp-alerts-sink/types"
)

type Publisher interface {
	Name() string

	Publish(
		ctx context.Context,
		source string,
		alert *types.AlertmanagerAlert,
		previous Results,
	) (Metadata, error)
}

type Metadata map[string]any

type Result struct {
	Status   string   `json:"status"`
	Metadata Metadata `json:"metadata,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type Results map[string]Result

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
