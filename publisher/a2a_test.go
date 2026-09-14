package publisher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/flashbots/amp-alerts-sink/config"
	mock_db "github.com/flashbots/amp-alerts-sink/mock/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestA2APublisherSendsTemplatedPromptAndStructuredDetails(t *testing.T) {
	ctrl := gomock.NewController(t)
	database := mock_db.NewMockDB(ctrl)

	var received *a2a.SendMessageRequest
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		if r.URL.Path == "/.well-known/agent-card.json" {
			require.NoError(t, json.NewEncoder(w).Encode(&a2a.AgentCard{
				Name: "test agent",
				SupportedInterfaces: []*a2a.AgentInterface{
					a2a.NewAgentInterface(server.URL+"/a2a", a2a.TransportProtocolJSONRPC),
				},
			}))
			return
		}

		require.Equal(t, "/a2a", r.URL.Path)
		var request struct {
			JSONRPC string                 `json:"jsonrpc"`
			ID      string                 `json:"id"`
			Method  string                 `json:"method"`
			Params  a2a.SendMessageRequest `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, "2.0", request.JSONRPC)
		assert.Equal(t, "SendMessage", request.Method)
		received = &request.Params

		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"task": map[string]any{
					"id":        "task-123",
					"contextId": "context-123",
					"status": map[string]any{
						"state": "TASK_STATE_SUBMITTED",
					},
				},
			},
		}))
	}))
	defer server.Close()

	cfg := &config.A2A{
		PromptUrl:   server.URL,
		BearerToken: "test-token",
		PromptTemplate: "{{ .Alert.Status }} {{ .Alert.Labels.alertname }} " +
			"in Slack message {{ .Publishers.slack.Metadata.message_ts }} from {{ .Source }}",
		Timeout: time.Second,
	}
	p, err := NewA2A(cfg, database)
	require.NoError(t, err)

	previous := Results{
		NameSlack: {
			Status: ResultStatusSucceeded,
			Metadata: Metadata{
				"channel_id": "C123",
				"message_ts": "1712345.6789",
				"thread_ts":  "1712345.0000",
			},
		},
	}
	database.EXPECT().Get(gomock.Any(), alertFiring.MessageDedupKey()).Return("", nil)
	database.EXPECT().Lock(gomock.Any(), alertFiring.MessageDedupKey(), timeoutLock).Return(true, nil)
	database.EXPECT().
		Set(gomock.Any(), alertFiring.MessageDedupKey(), timeoutA2AExpiry, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ time.Duration, value string) error {
			var stored Metadata
			require.NoError(t, json.Unmarshal([]byte(value), &stored))
			assert.Equal(t, "task-123", stored["task_id"])
			return nil
		})

	metadata, err := p.Publish(
		context.Background(),
		"test-source",
		alertFiring,
		previous,
	)
	require.NoError(t, err)
	assert.Equal(t, "task-123", metadata["task_id"])
	assert.Equal(t, "context-123", metadata["context_id"])
	assert.Equal(t, "TASK_STATE_SUBMITTED", metadata["state"])

	require.NotNil(t, received)
	require.NotNil(t, received.Message)
	assert.Equal(t, a2a.MessageRoleUser, received.Message.Role)
	require.Len(t, received.Message.Parts, 2)
	assert.Equal(
		t,
		"firing TestAlert in Slack message 1712345.6789 from test-source",
		received.Message.Parts[0].Text(),
	)
	assert.Equal(t, "application/json", received.Message.Parts[1].MediaType)

	details, ok := received.Message.Parts[1].Data().(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-source", details["source"])
	alert, ok := details["alert"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "firing", alert["status"])
	publishers, ok := details["publishers"].(map[string]any)
	require.True(t, ok)
	_, hasSlack := publishers[NameSlack]
	assert.True(t, hasSlack)
}

func TestA2APublisherReturnsStoredMetadataForDuplicate(t *testing.T) {
	ctrl := gomock.NewController(t)
	database := mock_db.NewMockDB(ctrl)
	p, err := NewA2A(&config.A2A{
		PromptUrl:      "https://agent.example.com",
		PromptTemplate: "test",
	}, database)
	require.NoError(t, err)

	database.EXPECT().
		Get(gomock.Any(), alertFiring.MessageDedupKey()).
		Return(`{"task_id":"task-123"}`, nil)
	database.EXPECT().
		Get(gomock.Any(), alertFiring.MessageDedupKey()).
		Return(`{"task_id":"task-123"}`, nil)

	metadata, err := p.Publish(
		context.Background(), "test-source", alertFiring, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "task-123", metadata["task_id"])
	assert.Equal(t, true, metadata["duplicate"])
}

func TestA2APublisherWaitsForSkippedDependency(t *testing.T) {
	p, err := NewA2A(&config.A2A{
		PromptUrl:      "https://agent.example.com",
		PromptTemplate: "test",
	}, nil)
	require.NoError(t, err)

	metadata, err := p.Publish(
		context.Background(),
		"test-source",
		alertFiring,
		Results{NameSlack: {Status: ResultStatusAlreadyLocked}},
	)

	assert.ErrorIs(t, err, ErrAlreadyLocked)
	assert.Equal(t, NameSlack, metadata["waiting_for"])
}

func TestNewA2ARejectsInvalidPromptTemplate(t *testing.T) {
	_, err := NewA2A(&config.A2A{
		PromptUrl:      "https://agent.example.com",
		PromptTemplate: "{{",
	}, nil)

	assert.ErrorContains(t, err, "failed to parse A2A prompt template")
}
