package db

import (
	"context"
	"errors"
	"time"

	"github.com/flashbots/amp-alerts-sink/config"
)

type DB interface {
	Lock(ctx context.Context, key string, expireIn time.Duration) (bool, error)
	Set(ctx context.Context, key string, expireIn time.Duration, value string) error
	Get(ctx context.Context, key string) (string, error)

	WithNamespace(namespace string) DB
}

var (
	ErrDbUndefined = errors.New("no database defined")
)

func New(cfg *config.DynamoDB) (DB, error) {
	switch {
	case cfg.Name != "":
		return newDynamoDb(cfg.Name)
	}

	return nil, ErrDbUndefined
}
