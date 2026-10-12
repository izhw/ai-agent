// Package eino adapts Eino ADK agents to the stable runtime SPI.
package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/schema"

	"github.com/izhw/ai-agent/contract"
	runtimespi "github.com/izhw/ai-agent/spi/runtime"
	storagespi "github.com/izhw/ai-agent/spi/storage"
	toolspi "github.com/izhw/ai-agent/spi/tool"
)

type DeepPreset struct {
	MaxIterations          int
	WithoutWriteTodos      bool
	WithoutGeneralSubAgent bool
	SubAgents              []contract.AgentSpec
}

type Dependencies struct {
	Models        ModelResolver
	Tools         toolspi.Resolver
	Checkpoints   storagespi.CheckpointStore
	CheckpointTTL time.Duration
	DeepPresets   map[string]DeepPreset
}

type Factory struct {
	runtimeType contract.RuntimeType
	deps        Dependencies
}

func NewChatModelFactory(deps Dependencies) *Factory {
	return &Factory{runtimeType: contract.RuntimeChatModel, deps: deps}
}

func NewDeepAgentFactory(deps Dependencies) *Factory {
	return &Factory{runtimeType: contract.RuntimeDeepAgent, deps: deps}
}

func (f *Factory) Type() contract.RuntimeType { return f.runtimeType }

func (f *Factory) Validate(ctx context.Context, spec contract.AgentSpec) error {
	if spec.ModelRef == "" {
		return errors.New("eino adapter: model_ref is required")
	}
	if spec.Runtime.Type != f.runtimeType {
		return fmt.Errorf(
			"eino adapter: factory %s cannot build %s",
			f.runtimeType,
			spec.Runtime.Type,
		)
	}
	if spec.Limits.MaxSteps < 0 {
		return errors.New("eino adapter: max_steps cannot be negative")
	}
	if f.deps.Models == nil {
		return errors.New("eino adapter: model resolver is required")
	}
	if _, err := f.deps.Models.Resolve(ctx, spec.ModelRef); err != nil {
		return err
	}
	if len(spec.ToolRefs) > 0 {
		if f.deps.Tools == nil {
			return errors.New("eino adapter: tool resolver is required")
		}
		if _, err := f.deps.Tools.Resolve(ctx, spec.ToolRefs); err != nil {
			return err
		}
	}
	if f.runtimeType == contract.RuntimeDeepAgent && spec.Runtime.PresetRef != "" {
		if _, ok := f.deps.DeepPresets[spec.Runtime.PresetRef]; !ok {
			return fmt.Errorf("eino adapter: deep preset not found: %s", spec.Runtime.PresetRef)
		}
	}
	return nil
}

func (f *Factory) Build(
	ctx context.Context,
	input runtimespi.BuildInput,
) (runtimespi.Runtime, error) {
	if err := f.Validate(ctx, input.Spec); err != nil {
		return nil, err
	}
	depth, hasDepth := ctx.Value(buildDepthKey{}).(int)
	if !hasDepth {
		depth = 0
	}
	if depth >= 8 {
		return nil, errors.New("eino: nested agent depth exceeds 8")
	}
	ctx = context.WithValue(ctx, buildDepthKey{}, depth+1)
	if f.deps.Models == nil {
		return nil, errors.New("eino adapter: model resolver is required")
	}
	chatModel, err := f.deps.Models.Resolve(ctx, input.Spec.ModelRef)
	if err != nil {
		return nil, err
	}
	var tools []toolspi.Tool
	if len(input.Spec.ToolRefs) > 0 {
		if f.deps.Tools == nil {
			return nil, errors.New("eino adapter: tool resolver is required")
		}
		tools, err = f.deps.Tools.Resolve(ctx, input.Spec.ToolRefs)
		if err != nil {
			return nil, err
		}
	}
	toolConfig := adk.ToolsConfig{}
	toolConfig.Tools = adaptTools(tools)
	maxSteps := input.Spec.Limits.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 20
	}
	var agent adk.Agent
	switch f.runtimeType {
	case contract.RuntimeChatModel:
		agent, err = adk.NewChatModelAgent(
			ctx,
			&adk.ChatModelAgentConfig{
				Name:          input.Spec.Key,
				Description:   input.Spec.Description,
				Instruction:   input.Spec.Instruction,
				Model:         chatModel,
				ToolsConfig:   toolConfig,
				MaxIterations: maxSteps,
				GenModelInput: literalInstruction,
			},
		)
	case contract.RuntimeDeepAgent:
		preset := f.deps.DeepPresets[input.Spec.Runtime.PresetRef]
		if preset.MaxIterations > 0 && input.Spec.Limits.MaxSteps == 0 {
			maxSteps = preset.MaxIterations
		}
		var children []adk.Agent
		for _, spec := range preset.SubAgents {
			if spec.Runtime.Type == "" {
				spec.Runtime.Type = contract.RuntimeChatModel
			}
			childFactory := &Factory{runtimeType: spec.Runtime.Type, deps: f.deps}
			child, buildErr := childFactory.Build(ctx, runtimespi.BuildInput{Spec: spec})
			if buildErr != nil {
				return nil, buildErr
			}
			childRuntime, ok := child.(*agentRuntime)
			if !ok {
				return nil, errors.New("eino adapter: deep sub-agent must be an Eino runtime")
			}
			children = append(children, childRuntime.agent)
		}
		agent, err = deep.New(
			ctx,
			&deep.Config{
				Name:                   input.Spec.Key,
				Description:            input.Spec.Description,
				Instruction:            input.Spec.Instruction,
				ChatModel:              chatModel,
				ToolsConfig:            toolConfig,
				MaxIteration:           maxSteps,
				WithoutWriteTodos:      preset.WithoutWriteTodos,
				WithoutGeneralSubAgent: preset.WithoutGeneralSubAgent,
				SubAgents:              children,
			},
		)
	default:
		return nil, fmt.Errorf("eino adapter: unsupported runtime %s", f.runtimeType)
	}
	if err != nil {
		return nil, err
	}
	var cp adk.CheckPointStore
	if f.deps.Checkpoints != nil {
		cp = checkpointAdapter{store: f.deps.Checkpoints, ttl: f.deps.CheckpointTTL}
	}
	return &agentRuntime{
		agent: agent,
		runner: adk.NewRunner(
			ctx,
			adk.RunnerConfig{Agent: agent, EnableStreaming: true, CheckPointStore: cp},
		),
	}, nil
}

type agentRuntime struct {
	agent  adk.Agent
	runner *adk.Runner
}
type buildDepthKey struct{}

func literalInstruction(
	_ context.Context,
	instruction string,
	input *adk.AgentInput,
) ([]adk.Message, error) {
	messages := make([]adk.Message, 0, len(input.Messages)+1)
	if instruction != "" {
		messages = append(messages, schema.SystemMessage(instruction))
	}
	return append(messages, input.Messages...), nil
}

func (r *agentRuntime) Run(
	ctx context.Context,
	input runtimespi.RunInput,
) (runtimespi.EventStream, error) {
	ctx, cancel := context.WithCancel(contract.WithRun(ctx, input.Run))
	messages := make([]adk.Message, 0, len(input.Messages))
	for _, message := range input.Messages {
		messages = append(messages, toEinoMessage(message))
	}
	iter := r.runner.Run(
		ctx,
		messages,
		adk.WithCheckPointID(input.CheckpointID),
		adk.WithSessionValues(input.Variables),
	)
	return newEventStream(ctx, cancel, iter, input.CheckpointID), nil
}

func (r *agentRuntime) Resume(
	ctx context.Context,
	input runtimespi.ResumeInput,
) (runtimespi.EventStream, error) {
	ctx, cancel := context.WithCancel(contract.WithRun(ctx, input.Run))
	var iter *adk.AsyncIterator[*adk.AgentEvent]
	var err error
	if len(input.Targets) > 0 {
		iter, err = r.runner.ResumeWithParams(
			ctx,
			input.CheckpointID,
			&adk.ResumeParams{Targets: input.Targets},
		)
	} else {
		iter, err = r.runner.Resume(ctx, input.CheckpointID)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	return newEventStream(ctx, cancel, iter, input.CheckpointID), nil
}
func (*agentRuntime) Close() error { return nil }

func toEinoMessage(message contract.Message) *schema.Message {
	switch message.Role {
	case contract.RoleSystem:
		return schema.SystemMessage(message.Content)
	case contract.RoleAssistant:
		return schema.AssistantMessage(message.Content, nil)
	case contract.RoleTool:
		return schema.ToolMessage(message.Content, message.ToolCallID)
	default:
		return schema.UserMessage(message.Content)
	}
}

type eventStream struct {
	items         chan streamItem
	cancel        context.CancelFunc
	once          sync.Once
	iter          *adk.AsyncIterator[*adk.AgentEvent]
	messageStream *schema.StreamReader[*schema.Message]
	streamRole    schema.RoleType
	toolName      string
	agentName     string
	checkpointID  string
	pending       []contract.RunEvent
	closed        bool
	mu            sync.Mutex
	frames        []*schema.Message
}

type streamItem struct {
	event contract.RunEvent
	err   error
}

func newEventStream(
	ctx context.Context,
	cancel context.CancelFunc,
	iter *adk.AsyncIterator[*adk.AgentEvent],
	checkpointID string,
) *eventStream {
	s := &eventStream{items: make(chan streamItem, 16), cancel: cancel}
	raw := &eventStream{iter: iter, checkpointID: checkpointID}
	go func() {
		defer close(s.items)
		defer raw.closeRaw()
		for {
			event, err := raw.nextRaw(ctx)
			if errors.Is(err, io.EOF) {
				return
			}
			select {
			case s.items <- streamItem{event, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *eventStream) Next(ctx context.Context) (contract.RunEvent, error) {
	select {
	case <-ctx.Done():
		s.cancel()
		return contract.RunEvent{}, ctx.Err()
	case item, ok := <-s.items:
		if !ok {
			return contract.RunEvent{}, io.EOF
		}
		return item.event, item.err
	}
}
func (s *eventStream) Close() error { s.once.Do(s.cancel); return nil }

func (s *eventStream) nextRaw(ctx context.Context) (contract.RunEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return contract.RunEvent{}, err
		}
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			return event, nil
		}
		if s.messageStream != nil {
			event, emitted, err := s.nextMessage()
			if err != nil || emitted {
				return event, err
			}
			continue
		}
		event, emitted, err := s.nextAgentEvent()
		if err != nil || emitted {
			return event, err
		}
	}
}

func (s *eventStream) nextMessage() (contract.RunEvent, bool, error) {
	message, err := s.messageStream.Recv()
	if errors.Is(err, io.EOF) {
		s.messageStream.Close()
		s.messageStream = nil
		if len(s.frames) == 0 {
			return contract.RunEvent{}, false, nil
		}
		full, concatErr := schema.ConcatMessages(s.frames)
		s.frames = nil
		if concatErr != nil {
			return contract.RunEvent{}, false, concatErr
		}
		if enqueueErr := s.enqueueCalls(full, s.agentName); enqueueErr != nil {
			return contract.RunEvent{}, false, enqueueErr
		}
		if len(s.pending) > 0 {
			return contract.RunEvent{}, false, nil
		}
		if s.streamRole == schema.Tool {
			return mapCompletedMessage(full, s.streamRole, s.toolName, s.agentName), true, nil
		}
		return contract.RunEvent{}, false, nil
	}
	if err != nil {
		return contract.RunEvent{}, false, err
	}
	if message == nil {
		return contract.RunEvent{}, false, nil
	}
	s.frames = append(s.frames, message)
	if s.streamRole != schema.Tool && message.Content != "" {
		return mapStreamingMessage(message, s.streamRole, s.toolName, s.agentName), true, nil
	}
	return contract.RunEvent{}, false, nil
}

func (s *eventStream) nextAgentEvent() (contract.RunEvent, bool, error) {
	event, ok := s.iter.Next()
	if !ok {
		return contract.RunEvent{}, true, io.EOF
	}
	if event == nil {
		return contract.RunEvent{}, false, nil
	}
	if event.Err != nil {
		return contract.RunEvent{}, true, event.Err
	}
	if event.Action != nil && event.Action.Interrupted != nil {
		return s.approvalEvent(event)
	}
	if event.Output == nil || event.Output.MessageOutput == nil {
		return contract.RunEvent{}, false, nil
	}
	output := event.Output.MessageOutput
	if output.IsStreaming {
		s.messageStream = output.MessageStream
		s.streamRole = output.Role
		s.toolName = output.ToolName
		s.agentName = event.AgentName
		return contract.RunEvent{}, false, nil
	}
	if output.Message == nil {
		return contract.RunEvent{}, false, nil
	}
	mapped := mapCompletedMessage(output.Message, output.Role, output.ToolName, event.AgentName)
	if enqueueErr := s.enqueueCalls(output.Message, event.AgentName); enqueueErr != nil {
		return contract.RunEvent{}, true, enqueueErr
	}
	if mapped.Content == "" && len(s.pending) > 0 {
		return contract.RunEvent{}, false, nil
	}
	return mapped, true, nil
}

func (s *eventStream) approvalEvent(event *adk.AgentEvent) (contract.RunEvent, bool, error) {
	points := make([]contract.ApprovalPoint, 0, len(event.Action.Interrupted.InterruptContexts))
	for _, point := range event.Action.Interrupted.InterruptContexts {
		if point.IsRootCause {
			points = append(
				points,
				contract.ApprovalPoint{ID: point.ID, Info: fmt.Sprint(point.Info)},
			)
		}
	}
	data, err := json.Marshal(map[string]any{"checkpoint_id": s.checkpointID, "points": points})
	if err != nil {
		return contract.RunEvent{}, true, err
	}
	return contract.RunEvent{
		Type:      contract.EventApprovalRequired,
		AgentName: event.AgentName,
		Data:      data,
	}, true, nil
}

func (s *eventStream) closeRaw() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.messageStream != nil {
		s.messageStream.Close()
	}
}

func (s *eventStream) enqueueCalls(message *schema.Message, agent string) error {
	for _, call := range message.ToolCalls {
		data, err := json.Marshal(map[string]any{"call_id": call.ID})
		if err != nil {
			return err
		}
		s.pending = append(
			s.pending,
			contract.RunEvent{
				Type:      contract.EventToolStarted,
				AgentName: agent,
				ToolName:  call.Function.Name,
				Data:      data,
			},
		)
	}
	return nil
}

func mapStreamingMessage(
	message *schema.Message,
	role schema.RoleType,
	toolName, agentName string,
) contract.RunEvent {
	if role == "" {
		role = message.Role
	}
	event := contract.RunEvent{
		AgentName: agentName,
		Role:      contract.Role(role),
		Content:   message.Content,
		ToolName:  toolName,
	}
	event.Type = contract.EventMessageDelta
	return event
}

func mapCompletedMessage(
	message *schema.Message,
	role schema.RoleType,
	toolName, agentName string,
) contract.RunEvent {
	if role == "" {
		role = message.Role
	}
	event := contract.RunEvent{
		AgentName: agentName,
		Role:      contract.Role(role),
		Content:   message.Content,
		ToolName:  toolName,
	}
	if message.ResponseMeta != nil && message.ResponseMeta.Usage != nil {
		u := message.ResponseMeta.Usage
		event.Usage = &contract.Usage{
			InputTokens:  u.PromptTokens,
			OutputTokens: u.CompletionTokens,
			TotalTokens:  u.TotalTokens,
		}
	}
	if role == schema.Tool {
		event.Type = contract.EventToolCompleted
	} else {
		event.Type = contract.EventMessageCompleted
	}
	return event
}
