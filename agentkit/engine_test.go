package agentkit_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/izhw/ai-agent/adapter/memory/portable"
	memoryadapter "github.com/izhw/ai-agent/adapter/storage/memory"
	"github.com/izhw/ai-agent/agentkit"
	"github.com/izhw/ai-agent/contract"
	runtimespi "github.com/izhw/ai-agent/spi/runtime"
)

type fakeFactory struct{ kind contract.RuntimeType }

func (f fakeFactory) Type() contract.RuntimeType                           { return f.kind }
func (fakeFactory) Validate(_ context.Context, _ contract.AgentSpec) error { return nil }
func (f fakeFactory) Build(_ context.Context, _ runtimespi.BuildInput) (runtimespi.Runtime, error) {
	return fakeRuntime(f), nil
}

type fakeRuntime struct{ kind contract.RuntimeType }

func (f fakeRuntime) Run(_ context.Context, _ runtimespi.RunInput) (runtimespi.EventStream, error) {
	return &runtimespi.SliceStream{
		Events: []contract.RunEvent{
			{
				Type:    contract.EventMessageCompleted,
				Role:    contract.RoleAssistant,
				Content: "response from " + string(f.kind),
			},
		},
	}, nil
}

func (fakeRuntime) Resume(
	_ context.Context,
	_ runtimespi.ResumeInput,
) (runtimespi.EventStream, error) {
	return &runtimespi.SliceStream{
		Events: []contract.RunEvent{
			{
				Type:    contract.EventMessageCompleted,
				Role:    contract.RoleAssistant,
				Content: "resumed",
			},
		},
	}, nil
}
func (fakeRuntime) Close() error { return nil }

func TestEngineCanSwitchRuntimeAndRemember(t *testing.T) {
	ctx := context.Background()
	store := memoryadapter.New()
	memoryService := portable.NewService(store.Memories(), nil)
	registry, err := runtimespi.NewRegistry(
		fakeFactory{contract.RuntimeChatModel},
		fakeFactory{contract.RuntimeDeepAgent},
	)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := agentkit.New(
		agentkit.WithStorage(store),
		agentkit.WithRuntimes(registry),
		agentkit.WithMemory(memoryService),
	)
	if err != nil {
		t.Fatal(err)
	}
	spec := contract.AgentSpec{
		TenantID:  "tenant-a",
		ProductID: "product-a",
		Key:       "assistant",
		Name:      "Assistant",
		ModelRef:  "fake",
		Runtime:   contract.RuntimeSpec{Type: contract.RuntimeChatModel},
		Memory:    contract.MemoryPolicy{Mode: contract.MemoryReadWrite, RequireExplicit: true},
	}
	first, err := engine.PublishAgent(ctx, spec, true)
	if err != nil {
		t.Fatal(err)
	}
	run, stream, err := engine.Start(
		ctx,
		contract.RunRequest{
			TenantID:  "tenant-a",
			ProductID: "product-a",
			UserID:    "user-a",
			AgentKey:  "assistant",
			Message:   contract.Message{Content: "请记住：我喜欢简洁回答"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	events := readAll(ctx, t, stream)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[1].Content != "response from chat_model" {
		t.Fatalf("unexpected response %q", events[1].Content)
	}
	stored, err := engine.GetRun(ctx, "tenant-a", "product-a", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != contract.RunCompleted {
		t.Fatalf("unexpected status %s", stored.Status)
	}
	memories, err := store.Memories().
		ListCandidates(ctx, "tenant-a", "product-a", "user-a", "assistant", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].Content != "我喜欢简洁回答" {
		t.Fatalf("unexpected memories %#v", memories)
	}
	spec.ID = ""
	spec.Version = first.Version + 1
	spec.Runtime.Type = contract.RuntimeDeepAgent
	second, err := engine.PublishAgent(ctx, spec, true)
	if err != nil {
		t.Fatal(err)
	}
	run2, stream2, err := engine.Start(
		ctx,
		contract.RunRequest{
			TenantID:  "tenant-a",
			ProductID: "product-a",
			UserID:    "user-a",
			AgentKey:  "assistant",
			Message:   contract.Message{Content: "你好"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stream2.Close()
	events = readAll(ctx, t, stream2)
	if run2.AgentVersion != second.Version {
		t.Fatal("run did not pin active version")
	}
	if events[1].Content != "response from deep_agent" {
		t.Fatalf("runtime was not switched: %q", events[1].Content)
	}
}

func TestTenantIsolation(t *testing.T) {
	ctx := context.Background()
	store := memoryadapter.New()
	registry, err := runtimespi.NewRegistry(fakeFactory{contract.RuntimeChatModel})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := agentkit.New(agentkit.WithStorage(store), agentkit.WithRuntimes(registry))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.PublishAgent(
		ctx,
		contract.AgentSpec{
			TenantID:  "a",
			ProductID: "p",
			Key:       "agent",
			ModelRef:  "fake",
			Runtime:   contract.RuntimeSpec{Type: contract.RuntimeChatModel},
		},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = engine.Start(
		ctx,
		contract.RunRequest{
			TenantID:  "b",
			ProductID: "p",
			UserID:    "u",
			AgentKey:  "agent",
			Message:   contract.Message{Content: "hello"},
		},
	); err == nil {
		t.Fatal("expected cross-tenant lookup to fail")
	}
}

func readAll(ctx context.Context, t *testing.T, stream runtimespi.EventStream) []contract.RunEvent {
	t.Helper()
	defer stream.Close()
	var result []contract.RunEvent
	for {
		event, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			return result
		}
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, event)
	}
}
