package eino

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/izhw/ai-agent/adapter/storage/memory"
	"github.com/izhw/ai-agent/contract"
	runtimespi "github.com/izhw/ai-agent/spi/runtime"
)

type fakeChatModel struct{}

func (fakeChatModel) Generate(
	_ context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	return schema.AssistantMessage("ok", nil), nil
}

func (fakeChatModel) Stream(
	_ context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("ok", nil)}), nil
}

func (f fakeChatModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

func TestFactoriesBuildChatAndDeepAgents(t *testing.T) {
	ctx := context.Background()
	models := NewModelRegistry()
	if err := models.Register("fake", fakeChatModel{}); err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	deps := Dependencies{
		Models:      models,
		Checkpoints: store.Checkpoints(),
		DeepPresets: map[string]DeepPreset{
			"minimal": {MaxIterations: 3, WithoutWriteTodos: true, WithoutGeneralSubAgent: true},
		},
	}
	tests := []struct {
		factory *Factory
		spec    contract.AgentSpec
	}{
		{
			factory: NewChatModelFactory(deps),
			spec: contract.AgentSpec{
				Key:      "chat",
				ModelRef: "fake",
				Runtime:  contract.RuntimeSpec{Type: contract.RuntimeChatModel},
			},
		},
		{
			factory: NewDeepAgentFactory(deps),
			spec: contract.AgentSpec{
				Key:      "deep",
				ModelRef: "fake",
				Runtime: contract.RuntimeSpec{
					Type:      contract.RuntimeDeepAgent,
					PresetRef: "minimal",
				},
			},
		},
	}
	for _, test := range tests {
		runtimeInstance, err := test.factory.Build(ctx, runtimespi.BuildInput{Spec: test.spec})
		if err != nil {
			t.Fatalf("build %s: %v", test.factory.Type(), err)
		}
		stream, err := runtimeInstance.Run(
			ctx,
			runtimespi.RunInput{
				Spec:         test.spec,
				Messages:     []contract.Message{{Role: contract.RoleUser, Content: "hello"}},
				CheckpointID: "cp",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for {
			event, nextErr := stream.Next(ctx)
			if errors.Is(nextErr, io.EOF) {
				break
			}
			if nextErr != nil {
				t.Fatal(nextErr)
			}
			if event.Content == "ok" {
				found = true
			}
		}
		_ = stream.Close()
		_ = runtimeInstance.Close()
		if !found {
			t.Fatalf("%s did not return model output", test.factory.Type())
		}
	}
}
