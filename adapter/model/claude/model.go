// Package claude creates native Anthropic Claude Eino chat models.
package claude

import (
	"context"
	"errors"
	"time"

	einoclaude "github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"

	secretsspi "github.com/izhw/ai-agent/spi/secrets"
)

const defaultMaxTokens = 4096

type Config struct {
	Model         string
	BaseURL       string
	CredentialRef string
	Timeout       time.Duration
	Temperature   *float32
	MaxTokens     *int
}

func New(
	ctx context.Context,
	config Config,
	secrets secretsspi.Resolver,
) (model.BaseChatModel, error) {
	if config.Model == "" {
		return nil, errors.New("claude adapter: model is required")
	}
	if secrets == nil {
		return nil, errors.New("claude adapter: secret resolver is required")
	}
	apiKey, err := secrets.Resolve(ctx, config.CredentialRef)
	if err != nil {
		return nil, err
	}
	maxTokens := defaultMaxTokens
	if config.MaxTokens != nil {
		maxTokens = *config.MaxTokens
	}
	var baseURL *string
	if config.BaseURL != "" {
		baseURL = &config.BaseURL
	}
	return einoclaude.NewChatModel(ctx, &einoclaude.Config{
		APIKey:         apiKey,
		BaseURL:        baseURL,
		Model:          config.Model,
		MaxTokens:      maxTokens,
		Temperature:    config.Temperature,
		RequestTimeout: config.Timeout,
	})
}
