package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ValidateArguments rejects undeclared keys, missing fields and type mismatches before execution.
func ValidateArguments(params map[string]Parameter, input json.RawMessage) error {
	var object map[string]any
	if len(input) > 1<<20 {
		return errors.New("tool arguments exceed 1 MiB")
	}
	if err := json.Unmarshal(input, &object); err != nil || object == nil {
		return errors.New("tool arguments must be a JSON object")
	}
	return validateObject(params, object)
}

func validateObject(params map[string]Parameter, object map[string]any) error {
	for name := range object {
		if _, ok := params[name]; !ok {
			return fmt.Errorf("undeclared argument: %s", name)
		}
	}
	for name, p := range params {
		v, ok := object[name]
		if !ok {
			if p.Required {
				return fmt.Errorf("missing argument: %s", name)
			}
			continue
		}
		if err := validateValue(p, v); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func validateValue(p Parameter, v any) error {
	valid := false
	switch p.Type {
	case "string":
		s, ok := v.(string)
		valid = ok
		if ok && len(p.Enum) > 0 {
			valid = false
			for _, e := range p.Enum {
				if s == e {
					valid = true
				}
			}
		}
	case "boolean":
		_, valid = v.(bool)
	case "number":
		_, valid = v.(float64)
	case "integer":
		n, ok := v.(float64)
		valid = ok && math.Trunc(n) == n
	case "object":
		m, ok := v.(map[string]any)
		valid = ok
		if ok {
			return validateObject(p.Properties, m)
		}
	case "array":
		items, ok := v.([]any)
		valid = ok
		if ok && p.Items != nil {
			for _, item := range items {
				if err := validateValue(*p.Items, item); err != nil {
					return err
				}
			}
		}
	case "null":
		valid = v == nil
	default:
		valid = false
	}
	if !valid {
		return fmt.Errorf("expected %s", p.Type)
	}
	return nil
}
