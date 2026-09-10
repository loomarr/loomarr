package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// openAIChatTools relocates complete inline schemas, never individual constraints.
// The envelope is per conversation, not mutable provider-wide state.
func openAIChatTools(messages []Message, schemas []ToolSchema) ([]openaiMessage, []openaiTool, map[string]bool, error) {
	envelopes := make(map[string]bool)
	for _, schema := range schemas {
		for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
			if _, present := schema.Parameters[keyword]; present {
				if schemaReferences(schema.Parameters) {
					return nil, nil, nil, fmt.Errorf("tool %q: cannot relocate schema references", schema.Name)
				}
				envelopes[schema.Name] = true
			}
		}
	}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.wireEnvelope {
				envelopes[call.Name] = true
			}
		}
	}
	wireTools := toOpenAITools(schemas)
	for i := range wireTools {
		if !envelopes[wireTools[i].Function.Name] {
			continue
		}
		wireTools[i].Function.Parameters = map[string]any{
			"type": "object", "properties": map[string]any{"input": wireTools[i].Function.Parameters},
			"required": []string{"input"}, "additionalProperties": false,
		}
	}
	wireMessages := toOpenAIMessages(messages)
	for i := range wireMessages {
		for j := range wireMessages[i].ToolCalls {
			call := &wireMessages[i].ToolCalls[j]
			if !envelopes[call.Function.Name] {
				continue
			}
			original := messages[i].ToolCalls[j].Arguments
			blob, err := json.Marshal(map[string]any{"input": original})
			if err != nil {
				return nil, nil, nil, fmt.Errorf("encode tool envelope: %w", err)
			}
			call.Function.Arguments = string(blob)
		}
	}
	return wireMessages, wireTools, envelopes, nil
}

func schemaReferences(value any) bool {
	blob, err := json.Marshal(value)
	// Marshaling handles typed slices/maps too; JSON keys have no whitespace here.
	return err != nil || bytes.Contains(blob, []byte(`"$ref":`)) || bytes.Contains(blob, []byte(`"$id":`))
}

func unwrapOpenAIToolCalls(calls []openaiToolCall, envelopes map[string]bool) []ToolCall {
	result := fromOpenAIToolCalls(calls)
	for i := range result {
		call := &result[i]
		if !envelopes[call.Name] {
			continue
		}
		call.wireEnvelope = true
		args, valid := call.Arguments["input"].(map[string]any)
		if !valid || len(call.Arguments) != 1 {
			call.Arguments = nil
			continue
		}
		call.Arguments = args
	}
	return result
}
