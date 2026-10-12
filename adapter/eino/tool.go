package eino

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	toolspi "github.com/izhw/ai-agent/spi/tool"
)

type toolAdapter struct{ delegate toolspi.Tool }

func adaptTools(values []toolspi.Tool) []einotool.BaseTool {
	result := make([]einotool.BaseTool, len(values))
	for i, value := range values {
		result[i] = &toolAdapter{delegate: value}
	}
	return result
}

func (t *toolAdapter) Info(context.Context) (*schema.ToolInfo, error) {
	desc := t.delegate.Descriptor()
	params := make(map[string]*schema.ParameterInfo, len(desc.Parameters))
	for name, param := range desc.Parameters {
		params[name] = adaptParameter(param)
	}
	info := &schema.ToolInfo{
		Name:  desc.Name,
		Desc:  desc.Description,
		Extra: map[string]any{"risk_level": desc.RiskLevel, "idempotent": desc.Idempotent},
	}
	if len(params) > 0 {
		info.ParamsOneOf = schema.NewParamsOneOfByParams(params)
	}
	return info, nil
}

func (t *toolAdapter) InvokableRun(
	ctx context.Context,
	arguments string,
	_ ...einotool.Option,
) (string, error) {
	if err := toolspi.ValidateArguments(
		t.delegate.Descriptor().Parameters,
		json.RawMessage(arguments),
	); err != nil {
		return "", err
	}
	if t.delegate.Descriptor().RiskLevel == "high" {
		interrupted, _, saved := compose.GetInterruptState[string](ctx)
		targeted, hasDecision, approved := compose.GetResumeContext[bool](ctx)
		if !interrupted || !targeted || !hasDecision {
			return "", compose.StatefulInterrupt(
				ctx,
				t.delegate.Descriptor().Name+": "+arguments,
				arguments,
			)
		}
		if !approved {
			return `{"status":"denied"}`, nil
		}
		if saved != arguments {
			return "", errors.New("tool approval arguments changed")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := t.delegate.Invoke(ctx, json.RawMessage(arguments))
	if err != nil {
		return "", err
	}
	if len(result) > 1<<20 {
		return "", errors.New("tool result exceeds 1 MiB")
	}
	if !json.Valid(result) {
		return "", errors.New("tool returned invalid JSON")
	}
	return string(result), nil
}

func adaptParameter(value toolspi.Parameter) *schema.ParameterInfo {
	result := &schema.ParameterInfo{
		Type:     schema.DataType(value.Type),
		Desc:     value.Description,
		Required: value.Required,
		Enum:     value.Enum,
	}
	if value.Items != nil {
		result.ElemInfo = adaptParameter(*value.Items)
	}
	if len(value.Properties) > 0 {
		result.SubParams = make(map[string]*schema.ParameterInfo, len(value.Properties))
		for name, child := range value.Properties {
			result.SubParams[name] = adaptParameter(child)
		}
	}
	return result
}
