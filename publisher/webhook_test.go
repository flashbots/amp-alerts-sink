package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/flashbots/amp-alerts-sink/config"
	mock_db "github.com/flashbots/amp-alerts-sink/mock/db"
	mock_publisher "github.com/flashbots/amp-alerts-sink/mock/publisher"
	"github.com/flashbots/amp-alerts-sink/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupWebhookPublisher(t *testing.T) (
	Publisher, *mock_db.MockDB, *mock_publisher.Mock_httpClient,
) {
	ctrl := gomock.NewController(t)
	db := mock_db.NewMockDB(ctrl)
	httpClient := mock_publisher.NewMock_httpClient(ctrl)

	wh := NewWebhook(&config.Webhook{
		URL:      "https://example.com/webhook",
		Method:   "POST",
		SendBody: true,
	}, db)

	_webhook := wh.(*webhook)
	_webhook.client = httpClient

	return wh, db, httpClient
}

func TestWebhookSuccessfulPublish(t *testing.T) {
	p, db, httpClient := setupWebhookPublisher(t)
	ctx := context.Background()
	alert := alertFiring

	// Check for duplicate
	db.EXPECT().
		Get(ctx, alert.MessageDedupKey()).
		Return("", nil)

	// Lock the alert
	db.EXPECT().
		Lock(ctx, alert.MessageDedupKey(), timeoutLock).
		Return(true, nil)

	// Send webhook
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "POST", req.Method)
			assert.Equal(t, "https://example.com/webhook", req.URL.String())
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

			// Verify the body
			body, err := io.ReadAll(req.Body)
			assert.NoError(t, err)

			var payload types.AlertmanagerWebhook
			err = json.Unmarshal(body, &payload)
			assert.NoError(t, err)
			assert.Equal(t, "4", payload.Version)
			assert.Equal(t, alert.MessageDedupKey(), payload.GroupKey)
			assert.Equal(t, "testSource", payload.Receiver)
			assert.Equal(t, "firing", payload.Status)
			assert.Len(t, payload.Alerts, 1)

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
			}, nil
		})

	// Mark as sent
	db.EXPECT().
		Set(ctx, alert.MessageDedupKey(), timeoutWebhookExpiry, "1").
		Return(nil)

	err := p.Publish(ctx, "testSource", alert)
	assert.NoError(t, err)
}

func TestWebhookWithoutBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := mock_db.NewMockDB(ctrl)
	httpClient := mock_publisher.NewMock_httpClient(ctrl)

	wh := NewWebhook(&config.Webhook{
		URL:      "https://example.com/webhook",
		Method:   "GET",
		SendBody: false,
	}, db)

	_webhook := wh.(*webhook)
	_webhook.client = httpClient

	ctx := context.Background()
	alert := alertFiring

	// Check for duplicate
	db.EXPECT().
		Get(ctx, alert.MessageDedupKey()).
		Return("", nil)

	// Lock the alert
	db.EXPECT().
		Lock(ctx, alert.MessageDedupKey(), timeoutLock).
		Return(true, nil)

	// Send webhook without body
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "GET", req.Method)
			assert.Equal(t, "https://example.com/webhook", req.URL.String())
			assert.Empty(t, req.Header.Get("Content-Type"))

			// Verify no body
			if req.Body != nil {
				body, err := io.ReadAll(req.Body)
				assert.NoError(t, err)
				assert.Empty(t, body)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("")),
			}, nil
		})

	// Mark as sent
	db.EXPECT().
		Set(ctx, alert.MessageDedupKey(), timeoutWebhookExpiry, "1").
		Return(nil)

	err := wh.Publish(ctx, "testSource", alert)
	assert.NoError(t, err)
}

func TestWebhookDuplicateAlert(t *testing.T) {
	p, db, _ := setupWebhookPublisher(t)
	ctx := context.Background()
	alert := alertFiring

	// Alert already sent
	db.EXPECT().
		Get(ctx, alert.MessageDedupKey()).
		Return("1", nil)

	err := p.Publish(ctx, "testSource", alert)
	assert.NoError(t, err)
}

func TestWebhookAlreadyLocked(t *testing.T) {
	p, db, _ := setupWebhookPublisher(t)
	ctx := context.Background()
	alert := alertFiring

	// Check for duplicate
	db.EXPECT().
		Get(ctx, alert.MessageDedupKey()).
		Return("", nil)

	// Lock fails - another instance is processing
	db.EXPECT().
		Lock(ctx, alert.MessageDedupKey(), timeoutLock).
		Return(false, nil)

	err := p.Publish(ctx, "testSource", alert)
	assert.Equal(t, ErrAlreadyLocked, err)
}

func TestWebhookHTTPError(t *testing.T) {
	p, db, httpClient := setupWebhookPublisher(t)
	ctx := context.Background()
	alert := alertFiring

	// Check for duplicate
	db.EXPECT().
		Get(ctx, alert.MessageDedupKey()).
		Return("", nil)

	// Lock the alert
	db.EXPECT().
		Lock(ctx, alert.MessageDedupKey(), timeoutLock).
		Return(true, nil)

	// Send webhook - returns error
	httpClient.EXPECT().
		Do(gomock.Any()).
		Return(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal server error"}`)),
		}, nil)

	err := p.Publish(ctx, "testSource", alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook returned status 500")
}

func TestWebhookNetworkError(t *testing.T) {
	p, db, httpClient := setupWebhookPublisher(t)
	ctx := context.Background()
	alert := alertFiring

	// Check for duplicate
	db.EXPECT().
		Get(ctx, alert.MessageDedupKey()).
		Return("", nil)

	// Lock the alert
	db.EXPECT().
		Lock(ctx, alert.MessageDedupKey(), timeoutLock).
		Return(true, nil)

	// Send webhook - network error
	httpClient.EXPECT().
		Do(gomock.Any()).
		Return(nil, assert.AnError)

	err := p.Publish(ctx, "testSource", alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook request failed")
}

func TestWebhookDifferentMethods(t *testing.T) {
	testCases := []string{
		"POST",
		"PUT",
		"PATCH",
		"DELETE",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := mock_db.NewMockDB(ctrl)
			httpClient := mock_publisher.NewMock_httpClient(ctrl)

			wh := NewWebhook(&config.Webhook{
				URL:      "https://example.com/webhook",
				Method:   tc,
				SendBody: true,
			}, db)

			_webhook := wh.(*webhook)
			_webhook.client = httpClient

			ctx := context.Background()
			alert := alertFiring

			// Check for duplicate
			db.EXPECT().
				Get(ctx, alert.MessageDedupKey()).
				Return("", nil)

			// Lock the alert
			db.EXPECT().
				Lock(ctx, alert.MessageDedupKey(), timeoutLock).
				Return(true, nil)

			// Send webhook with specified method
			httpClient.EXPECT().
				Do(gomock.Any()).
				DoAndReturn(func(req *http.Request) (*http.Response, error) {
					assert.Equal(t, tc, req.Method)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString("")),
					}, nil
				})

			// Mark as sent
			db.EXPECT().
				Set(ctx, alert.MessageDedupKey(), timeoutWebhookExpiry, "1").
				Return(nil)

			err := wh.Publish(ctx, "testSource", alert)
			assert.NoError(t, err)
		})
	}
}

func TestWebhookDefaultMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := mock_db.NewMockDB(ctrl)

	// Create webhook without specifying method
	wh := NewWebhook(&config.Webhook{
		URL:      "https://example.com/webhook",
		SendBody: true,
	}, db)

	_webhook := wh.(*webhook)
	assert.Equal(t, "POST", _webhook.method)
}

func TestWebhookResolvingAlert(t *testing.T) {
	p, db, httpClient := setupWebhookPublisher(t)
	ctx := context.Background()
	alert := alertResolved

	// Check for duplicate
	db.EXPECT().
		Get(ctx, alert.MessageDedupKey()).
		Return("", nil)

	// Lock the alert
	db.EXPECT().
		Lock(ctx, alert.MessageDedupKey(), timeoutLock).
		Return(true, nil)

	// Send webhook
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			// Verify the body contains resolved status
			body, err := io.ReadAll(req.Body)
			assert.NoError(t, err)

			var payload types.AlertmanagerWebhook
			err = json.Unmarshal(body, &payload)
			assert.NoError(t, err)
			assert.Equal(t, "resolved", payload.Status)
			assert.Len(t, payload.Alerts, 1)
			assert.Equal(t, "resolved", payload.Alerts[0].Status)

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
			}, nil
		})

	// Mark as sent
	db.EXPECT().
		Set(ctx, alert.MessageDedupKey(), timeoutWebhookExpiry, "1").
		Return(nil)

	err := p.Publish(ctx, "testSource", alert)
	assert.NoError(t, err)
}

func TestWebhookDBErrorContinuesWithSend(t *testing.T) {
	p, db, httpClient := setupWebhookPublisher(t)
	ctx := context.Background()
	alert := alertFiring

	// DB check fails but we continue
	db.EXPECT().
		Get(ctx, alert.MessageDedupKey()).
		Return("", assert.AnError)

	// Send webhook anyway
	httpClient.EXPECT().
		Do(gomock.Any()).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
		}, nil)

	// Mark as sent
	db.EXPECT().
		Set(ctx, alert.MessageDedupKey(), timeoutWebhookExpiry, "1").
		Return(nil)

	err := p.Publish(ctx, "testSource", alert)
	assert.NoError(t, err)
}
