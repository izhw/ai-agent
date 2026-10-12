// Package model provides an extensible factory for Eino chat model providers.
package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"

	modelclaude "github.com/izhw/ai-agent/adapter/model/claude"
	modelgemini "github.com/izhw/ai-agent/adapter/model/gemini"
	modelopenai "github.com/izhw/ai-agent/adapter/model/openai"
	secretsspi "github.com/izhw/ai-agent/spi/secrets"
)

type Config struct {
	Provider      string
	Model         string
	BaseURL       string
	APIVersion    string
	CredentialRef string
	Timeout       time.Duration
	Temperature   *float32
	MaxTokens     *int
}

type Builder func(context.Context, Config, secretsspi.Resolver) (einomodel.BaseChatModel, error)

type Factory struct {
	mu       sync.RWMutex
	builders map[string]Builder
}

func NewFactory() *Factory {
	factory := &Factory{builders: make(map[string]Builder)}
	factory.registerBuiltins()
	return factory
}

func (f *Factory) Register(provider string, builder Builder) error {
	provider = normalizeProvider(provider)
	if provider == "" || builder == nil {
		return errors.New("model provider: name and builder are required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.builders[provider]; exists {
		return fmt.Errorf("model provider: duplicate provider %q", provider)
	}
	f.builders[provider] = builder
	return nil
}

func (f *Factory) Build(
	ctx context.Context,
	config Config,
	secrets secretsspi.Resolver,
) (einomodel.BaseChatModel, error) {
	provider := normalizeProvider(config.Provider)
	f.mu.RLock()
	builder, ok := f.builders[provider]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("model provider: unsupported provider %q", config.Provider)
	}
	config.Provider = provider
	return builder(ctx, config, secrets)
}

func (f *Factory) registerBuiltins() {
	registerAliases(f, buildClaude, "claude", "anthropic")
	registerAliases(f, buildGemini, "gemini", "google")
	registerCompatible(f, "openai", "")
	registerCompatible(f, "openai-compatible", "")
	registerCompatible(f, "deepseek", "https://api.deepseek.com")
	registerCompatible(f, "doubao", "https://ark.cn-beijing.volces.com/api/v3")
	registerCompatible(f, "doubao-seed", "https://ark.cn-beijing.volces.com/api/v3")
	registerCompatible(f, "ark", "https://ark.cn-beijing.volces.com/api/v3")
	registerCompatible(f, "kimi", "https://api.moonshot.cn/v1")
	registerCompatible(f, "moonshot", "https://api.moonshot.cn/v1")
	registerCompatible(f, "qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	registerCompatible(f, "dashscope", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	registerCompatible(f, "glm", "https://open.bigmodel.cn/api/paas/v4")
	registerCompatible(f, "zhipu", "https://open.bigmodel.cn/api/paas/v4")
	registerCompatible(f, "minimax", "https://api.minimaxi.com/v1")
}

func registerAliases(factory *Factory, builder Builder, providers ...string) {
	for _, provider := range providers {
		if err := factory.Register(provider, builder); err != nil {
			panic(err)
		}
	}
}

func registerCompatible(factory *Factory, provider, defaultBaseURL string) {
	if err := factory.Register(provider, func(
		ctx context.Context,
		config Config,
		secrets secretsspi.Resolver,
	) (einomodel.BaseChatModel, error) {
		if config.BaseURL == "" {
			config.BaseURL = defaultBaseURL
		}
		if config.Provider == "openai-compatible" && config.BaseURL == "" {
			return nil, errors.New("model provider: openai-compatible requires base_url")
		}
		return modelopenai.New(ctx, modelopenai.Config{
			Model:         config.Model,
			BaseURL:       config.BaseURL,
			CredentialRef: config.CredentialRef,
			Timeout:       config.Timeout,
			Temperature:   config.Temperature,
			MaxTokens:     config.MaxTokens,
		}, secrets)
	}); err != nil {
		panic(err)
	}
}

func buildClaude(
	ctx context.Context,
	config Config,
	secrets secretsspi.Resolver,
) (einomodel.BaseChatModel, error) {
	return modelclaude.New(ctx, modelclaude.Config{
		Model:         config.Model,
		BaseURL:       config.BaseURL,
		CredentialRef: config.CredentialRef,
		Timeout:       config.Timeout,
		Temperature:   config.Temperature,
		MaxTokens:     config.MaxTokens,
	}, secrets)
}

func buildGemini(
	ctx context.Context,
	config Config,
	secrets secretsspi.Resolver,
) (einomodel.BaseChatModel, error) {
	return modelgemini.New(ctx, modelgemini.Config{
		Model:         config.Model,
		BaseURL:       config.BaseURL,
		APIVersion:    config.APIVersion,
		CredentialRef: config.CredentialRef,
		Timeout:       config.Timeout,
		Temperature:   config.Temperature,
		MaxTokens:     config.MaxTokens,
	}, secrets)
}

func normalizeProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	return strings.ReplaceAll(provider, "_", "-")
}
