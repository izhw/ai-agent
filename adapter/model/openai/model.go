// Package openai creates OpenAI-compatible Eino chat models.
package openai

import (
	"context"
	"errors"
	"time"

	einopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	secretsspi "github.com/izhw/ai-agent/spi/secrets"
)

type Config struct {
	Model         string
	BaseURL       string
	CredentialRef string
	Timeout       time.Duration
	Temperature   *float32
	MaxTokens     *int
	ByAzure       bool
	APIVersion    string
}

func New(
	ctx context.Context,
	config Config,
	secrets secretsspi.Resolver,
) (model.BaseChatModel, error) {
	if config.Model == "" {
		return nil, errors.New("openai adapter: model is required")
	}
	if secrets == nil {
		return nil, errors.New("openai adapter: secret resolver is required")
	}
	apiKey, err := secrets.Resolve(ctx, config.CredentialRef)
	if err != nil {
		return nil, err
	}
	return einopenai.NewChatModel(
		ctx,
		&einopenai.ChatModelConfig{
			APIKey:              apiKey,
			BaseURL:             config.BaseURL,
			Model:               config.Model,
			Timeout:             config.Timeout,
			Temperature:         config.Temperature,
			MaxCompletionTokens: config.MaxTokens,
			ByAzure:             config.ByAzure,
			APIVersion:          config.APIVersion,
		},
	)
}
