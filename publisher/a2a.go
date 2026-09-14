package publisher

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"text/template"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/flashbots/amp-alerts-sink/db"
	"github.com/flashbots/amp-alerts-sink/logutils"
	"github.com/flashbots/amp-alerts-sink/types"
	"go.uber.org/zap"
)

type a2aSender interface {
	SendMessage(context.Context, *a2a.SendMessageRequest) (a2a.SendMessageResult, error)
}

type a2aError struct {
	err error
}

func (err a2aError) Error() string {
	return err.err.Error()
}

func (err a2aError) IsReportable() bool {
	return false
}

type a2aClientFactory func(context.Context, string, string) (a2aSender, error)

type a2aPublisher struct {
	agentURL    string
	bearerToken string
	timeout     time.Duration
	prompt      *template.Template

	db db.DB

	clientMu      sync.Mutex
	client        a2aSender
	clientFactory a2aClientFactory
}

type a2aPromptData struct {
	Source     string                   `json:"source"`
	Alert      *types.AlertmanagerAlert `json:"alert"`
	Publishers Results                  `json:"publishers"`
}

func NewA2A(cfg *config.A2A, database db.DB) (Publisher, error) {
	promptTemplate := cfg.PromptTemplate
	if t, err := base64.StdEncoding.DecodeString(cfg.PromptTemplate); err == nil {
		promptTemplate = string(t)
	}
	prompt, err := template.New("a2a-prompt").
		Option("missingkey=error").
		Funcs(template.FuncMap{"toJSON": templateJSON}).
		Parse(promptTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse A2A prompt template: %w", err)
	}

	return &a2aPublisher{
		agentURL:      cfg.PromptUrl,
		bearerToken:   cfg.BearerToken,
		timeout:       cfg.Timeout,
		prompt:        prompt,
		db:            database,
		clientFactory: newA2AClient,
	}, nil
}

func (p *a2aPublisher) Name() string {
	return NameA2A
}

func (p *a2aPublisher) Publish(
	ctx context.Context,
	source string,
	alert *types.AlertmanagerAlert,
	previousPublishers Results,
) (Metadata, PublisherError) {
	l := logutils.LoggerFromContext(ctx)

	for dependency, result := range previousPublishers {
		if result.Status == ResultStatusAlreadyLocked {
			l.Info("Previous publisher is not finished yet",
				zap.String("dependency", dependency),
			)
			return Metadata{"waiting_for": dependency}, a2aError{err: ErrAlreadyLocked}
		}
	}

	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	isDup, err := p.checkDupAndLock(ctx, alert)
	if isDup {
		var metadata Metadata
		if v, err := p.db.Get(ctx, alert.MessageDedupKey()); err == nil {
			_ = json.Unmarshal([]byte(v), &metadata)
		}
		metadata["duplicate"] = true
		// A non-nil err here is always ErrAlreadyLocked, which the processor
		// now handles as a harmless duplicate (an HA peer will publish it).
		l.Info("Duplicate alert detected", zap.Error(err))
		return metadata, a2aError{err: err}
	}
	if err != nil {
		l.Error("Failed to check for duplicate alert, refusing to prompt a2a", zap.Error(err))
		return nil, a2aError{err: err}
	}

	// not a duplicate

	details := a2aPromptData{
		Alert:      alert,
		Publishers: previousPublishers,
		Source:     source,
	}
	prompt, err := p.renderPrompt(details)
	if err != nil {
		return nil, a2aError{err: err}
	}

	client, err := p.getClient(ctx)
	if err != nil {
		return nil, a2aError{err: err}
	}

	dataPart := a2a.NewDataPart(details)
	dataPart.MediaType = "application/json"
	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(prompt), dataPart)

	if p.bearerToken != "" {
		ctx = a2aclient.AttachServiceParams(ctx, a2aclient.ServiceParams{
			"Authorization": {"Bearer " + p.bearerToken},
		})
	}

	l.Info("Prompting A2A agent",
		zap.String("agent_url", p.agentURL),
		zap.String("message_id", message.ID),
		zap.String("prompt", prompt),
	)
	response, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: message})
	if err != nil {
		p.clearClient()
		return nil, a2aError{err: fmt.Errorf("failed to send A2A message: %w", err)}
	}

	metadata := a2aResponseMetadata(response)
	encodedMetadata, marshalErr := json.Marshal(metadata)
	if marshalErr != nil {
		l.Warn("Failed to encode A2A response metadata for deduplication",
			zap.Error(marshalErr),
		)
	} else if err := p.db.Set(ctx, alert.MessageDedupKey(), timeoutA2AExpiry, string(encodedMetadata)); err != nil {
		l.Warn("Failed to save A2A publication result", zap.Error(err))
	}

	l.Info(
		"Prompted A2A agent",
		zap.String("agent_url", p.agentURL),
		zap.String("message_id", message.ID),
		zap.String("prompt", prompt),
		zap.Any("response_metadata", metadata),
	)
	return metadata, nil
}

func (p *a2aPublisher) checkDupAndLock(
	ctx context.Context,
	alert *types.AlertmanagerAlert,
) (bool, error) {
	v, err := p.db.Get(ctx, alert.MessageDedupKey())
	if err != nil {
		return false, fmt.Errorf("failed to check for duplicate alert: %w", err)
	}
	if v != "" {
		return true, nil
	}

	didLock, err := p.db.Lock(ctx, alert.MessageDedupKey(), timeoutLock)
	if err != nil {
		return false, fmt.Errorf("failed to lock alert: %w", err)
	}
	if !didLock {
		// another instance is about to publish
		return true, ErrAlreadyLocked
	}

	return false, nil
}

func (p *a2aPublisher) renderPrompt(data a2aPromptData) (string, error) {
	var rendered bytes.Buffer
	if err := p.prompt.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("failed to render A2A prompt template: %w", err)
	}
	return rendered.String(), nil
}

func (p *a2aPublisher) getClient(ctx context.Context) (a2aSender, error) {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()

	if p.client != nil {
		return p.client, nil
	}

	client, err := p.clientFactory(ctx, p.agentURL, p.bearerToken)
	if err != nil {
		return nil, err
	}
	p.client = client
	return p.client, nil
}

func (p *a2aPublisher) clearClient() {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()
	p.client = nil
}

func newA2AClient(ctx context.Context, agentURL, bearerToken string) (a2aSender, error) {
	resolver := agentcard.NewResolver(http.DefaultClient)
	var resolveOptions []agentcard.ResolveOption
	if bearerToken != "" {
		resolveOptions = append(resolveOptions,
			agentcard.WithRequestHeader("Authorization", "Bearer "+bearerToken),
		)
	}

	card, err := resolver.Resolve(ctx, agentURL, resolveOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve A2A agent card: %w", err)
	}

	// some agent cards (e.g. kagent) advertise internal/cluster-local URLs in
	// their supported interfaces instead of public URL they were resolved from
	for _, iface := range card.SupportedInterfaces {
		iface.URL = agentURL
	}

	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		return nil, fmt.Errorf("failed to create A2A client: %w", err)
	}
	return client, nil
}

func a2aResponseMetadata(response a2a.SendMessageResult) Metadata {
	metadata := Metadata{}
	if response == nil {
		return metadata
	}

	taskInfo := response.TaskInfo()
	if taskInfo.TaskID != "" {
		metadata["task_id"] = string(taskInfo.TaskID)
	}
	if taskInfo.ContextID != "" {
		metadata["context_id"] = taskInfo.ContextID
	}
	if responseMetadata := response.Meta(); len(responseMetadata) > 0 {
		metadata["agent_metadata"] = responseMetadata
	}

	switch typed := response.(type) {
	case *a2a.Task:
		metadata["response_type"] = "task"
		metadata["state"] = typed.Status.State.String()
	case *a2a.Message:
		metadata["response_type"] = "message"
		metadata["message_id"] = typed.ID
	}

	return metadata
}

func templateJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
