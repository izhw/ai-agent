package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"

	secretsspi "github.com/izhw/ai-agent/spi/secrets"
)

func TestFactoryProviderAliases(t *testing.T) {
	factory := NewFactory()
	providers := []string{
		"openai", "openai-compatible", "claude", "anthropic", "gemini", "google",
		"deepseek", "doubao", "doubao-seed", "ark", "kimi", "moonshot", "qwen",
		"dashscope", "glm", "zhipu", "minimax",
	}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			builder, ok := factory.builders[provider]
			if !ok || builder == nil {
				t.Fatalf("provider %q is not registered", provider)
			}
		})
	}
}

func TestFactoryCustomProvider(t *testing.T) {
	factory := NewFactory()
	called := false
	wantErr := errors.New("custom builder called")
	err := factory.Register("enterprise_model", func(
		_ context.Context,
		config Config,
		_ secretsspi.Resolver,
	) (einomodel.BaseChatModel, error) {
		called = config.Provider == "enterprise-model"
		return nil, wantErr
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = factory.Build(
		context.Background(),
		Config{Provider: "enterprise_model"},
		nil,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("custom provider builder was not called")
	}
}

func TestFactoryRejectsUnsupportedProvider(t *testing.T) {
	_, err := NewFactory().Build(
		context.Background(),
		Config{Provider: "unknown"},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}
