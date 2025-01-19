package publisher

import (
	"context"
	"time"

	"github.com/flashbots/amp-alerts-sink/types"
)

type Publisher interface {
	Publish(ctx context.Context, source string, alert *types.AlertmanagerAlert) error
}

const (
	timeoutLock         = time.Second
	timeoutThreadExpiry = 30 * 24 * time.Hour
)
