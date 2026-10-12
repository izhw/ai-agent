// Package gemini creates native Google Gemini Eino chat models.
package gemini

import (
	"context"
	"errors"
	"time"

	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"

	secretsspi "github.com/izhw/ai-agent/spi/secrets"
)

type Config struct {
	Model         string
	BaseURL       string
	APIVersion    string
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
		return nil, errors.New("gemini adapter: model is required")
	}
	if secrets == nil {
		return nil, errors.New("gemini adapter: secret resolver is required")
	}
	apiKey, err := secrets.Resolve(ctx, config.CredentialRef)
	if err != nil {
		return nil, err
	}
	clientConfig := &genai.ClientConfig{
		APIKey: apiKey,
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    config.BaseURL,
			APIVersion: config.APIVersion,
		},
	}
	if config.Timeout > 0 {
		clientConfig.HTTPOptions.Timeout = &config.Timeout
	}
	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, err
	}
	return einogemini.NewChatModel(ctx, &einogemini.Config{
		Client:      client,
		Model:       config.Model,
		MaxTokens:   config.MaxTokens,
		Temperature: config.Temperature,
	})
}
