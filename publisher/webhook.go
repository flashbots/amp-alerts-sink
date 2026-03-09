package publisher

//go:generate go tool mockgen -package mock_publisher -destination ../mock/publisher/webhook.go -source webhook.go -mock_names httpClient=Mock_httpClient httpClient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/flashbots/amp-alerts-sink/db"
	"github.com/flashbots/amp-alerts-sink/logutils"
	"github.com/flashbots/amp-alerts-sink/types"
	"go.uber.org/zap"
)

type webhook struct {
	url      string
	method   string
	sendBody bool

	client httpClient
	db     db.DB
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewWebhook(cfg *config.Webhook, db db.DB) Publisher {
	method := cfg.Method
	if method == "" {
		method = "POST"
	}

	return &webhook{
		url:      cfg.URL,
		method:   method,
		sendBody: cfg.SendBody,

		client: http.DefaultClient,
		db:     db,
	}
}

func (w *webhook) Publish(
	ctx context.Context,
	source string,
	alert *types.AlertmanagerAlert,
) error {
	l := logutils.LoggerFromContext(ctx)
	l.Info("Publishing alert", zap.Any("alert", alert))

	isDup, err := w.checkDupAndLock(ctx, alert)
	if isDup {
		// Enter this branch even with non-nil err;
		// Only for ErrAlreadyLocked, so that lambda execution will be restarted
		l.Info("Duplicate alert detected", zap.Error(err))
		return err
	}
	// not a duplicate
	if err != nil {
		l.Error("Failed to check for duplicate alert, sending webhook", zap.Error(err))
	}

	err = w.sendWebhook(ctx, source, alert)
	if err != nil {
		l.Error("Failed to send alert", zap.Error(err))
	} else {
		// sent correctly, prevent other instances from sending
		_ = w.db.Set(ctx, alert.MessageDedupKey(), timeoutWebhookExpiry, "1")
	}
	return err
}

func (w *webhook) checkDupAndLock(ctx context.Context, alert *types.AlertmanagerAlert) (isDup bool, err error) {
	v, err := w.db.Get(ctx, alert.MessageDedupKey())
	if err != nil {
		return false, fmt.Errorf("failed to check for duplicate alert: %w", err)
	}
	if v != "" {
		return true, nil
	}

	didLock, err := w.db.Lock(ctx, alert.MessageDedupKey(), timeoutLock)
	if err != nil {
		return false, fmt.Errorf("failed to lock alert: %w", err)
	}
	if !didLock {
		// another instance is about to publish
		return true, ErrAlreadyLocked
	}

	return false, nil
}

func (w *webhook) sendWebhook(
	ctx context.Context,
	source string,
	alert *types.AlertmanagerAlert,
) error {
	l := logutils.LoggerFromContext(ctx)

	var reqBody io.Reader
	var contentType string

	if w.sendBody {
		buf, err := w.encodeAlert(source, alert)
		if err != nil {
			l.Error("Failed to encode alert", zap.Error(err))
			return err
		}
		l.Debug("Webhook payload", zap.ByteString("body", buf.Bytes()))

		reqBody = buf
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, w.method, w.url, reqBody)
	if err != nil {
		l.Error("Failed to create webhook request", zap.Error(err))
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	l.Info("Sending webhook request",
		zap.String("url", w.url),
		zap.String("method", w.method),
		zap.Bool("send_body", w.sendBody),
		zap.String("alert_fingerprint", alert.MessageDedupKey()),
	)

	resp, err := w.client.Do(req)
	if err != nil {
		l.Error("Webhook request failed", zap.Error(err))
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		l.Warn("Failed to read webhook response body", zap.Error(readErr))
	}

	if resp.StatusCode != http.StatusOK {
		l.Error("Webhook returned non-200 status",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(respBody)),
		)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, resp.Status)
	}

	l.Info("Successfully published alert to webhook",
		zap.String("response_body", string(respBody)),
	)
	return nil
}

func (w *webhook) encodeAlert(source string, alert *types.AlertmanagerAlert) (*bytes.Buffer, error) {
	body := types.AlertmanagerWebhook{
		Version:  "4",
		GroupKey: alert.MessageDedupKey(),

		AlertmanagerMessage: types.AlertmanagerMessage{
			Receiver: source,
			Status:   alert.Status,
			Alerts:   []types.AlertmanagerAlert{*alert},
		},
	}

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return nil, fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	return buf, nil
}
