package translator

import (
	"strings"
	"testing"
)

func TestTranslateClaudeChunkToOpenAI(t *testing.T) {
	state := &ClaudeToOpenAIStreamState{}

	t.Run("message_start emits role assistant chunk", func(t *testing.T) {
		payload := []byte(`{"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"union-alpha","usage":{"input_tokens":15,"output_tokens":0}}}`)
		out, err := TranslateClaudeChunkToOpenAI(payload, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		outStr := string(out)
		if !strings.Contains(outStr, `"role":"assistant"`) {
			t.Errorf("expected role assistant in output, got: %s", outStr)
		}
		if !strings.Contains(outStr, `"id":"chatcmpl-msg_123"`) {
			t.Errorf("expected chatcmpl ID, got: %s", outStr)
		}
	})

	t.Run("content_block_delta emits text content", func(t *testing.T) {
		payload := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello world"}}`)
		out, err := TranslateClaudeChunkToOpenAI(payload, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		outStr := string(out)
		if !strings.Contains(outStr, `"content":"hello world"`) {
			t.Errorf("expected content in output, got: %s", outStr)
		}
	})

	t.Run("thinking_delta emits reasoning_content", func(t *testing.T) {
		payload := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"thinking deep"}}`)
		out, err := TranslateClaudeChunkToOpenAI(payload, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		outStr := string(out)
		if !strings.Contains(outStr, `"reasoning_content":"thinking deep"`) {
			t.Errorf("expected reasoning_content in output, got: %s", outStr)
		}
	})

	t.Run("tool_use block start and delta", func(t *testing.T) {
		startPayload := []byte(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_abc","name":"calculator","input":{}}}`)
		out1, err := TranslateClaudeChunkToOpenAI(startPayload, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(out1), `"name":"calculator"`) {
			t.Errorf("expected tool name in start chunk, got: %s", string(out1))
		}

		deltaPayload := []byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"expr\":\"1+1\"}"}}`)
		out2, err := TranslateClaudeChunkToOpenAI(deltaPayload, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(out2), `{\"expr\":\"1+1\"}`) {
			t.Errorf("expected partial json in delta chunk, got: %s", string(out2))
		}
	})

	t.Run("message_delta emits finish_reason and usage", func(t *testing.T) {
		payload := []byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}`)
		out, err := TranslateClaudeChunkToOpenAI(payload, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		outStr := string(out)
		if !strings.Contains(outStr, `"finish_reason":"stop"`) {
			t.Errorf("expected finish_reason stop, got: %s", outStr)
		}
		if !strings.Contains(outStr, `"completion_tokens":20`) {
			t.Errorf("expected usage completion_tokens 20, got: %s", outStr)
		}
	})

	t.Run("message_stop emits [DONE]", func(t *testing.T) {
		payload := []byte(`{"type":"message_stop"}`)
		out, err := TranslateClaudeChunkToOpenAI(payload, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		outStr := string(out)
		if !strings.Contains(outStr, "data: [DONE]\n\n") {
			t.Errorf("expected [DONE], got: %s", outStr)
		}
	})
}

func TestTranslateClaudeResponseToOpenAI(t *testing.T) {
	claudeJSON := []byte(`{
		"id": "msg_xyz",
		"type": "message",
		"role": "assistant",
		"model": "union-alpha",
		"content": [
			{"type": "thinking", "thinking": "Let me calculate."},
			{"type": "text", "text": "The answer is 42."},
			{"type": "tool_use", "id": "call_1", "name": "echo", "input": {"val": 42}}
		],
		"stop_reason": "tool_use",
		"usage": {
			"input_tokens": 10,
			"output_tokens": 5,
			"cache_read_input_tokens": 2
		}
	}`)

	out, err := TranslateClaudeResponseToOpenAI(claudeJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, `"id":"chatcmpl-msg_xyz"`) {
		t.Errorf("expected chatcmpl ID, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"content":"The answer is 42."`) {
		t.Errorf("expected content, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"reasoning_content":"Let me calculate."`) {
		t.Errorf("expected reasoning_content, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"finish_reason":"tool_calls"`) {
		t.Errorf("expected finish_reason tool_calls, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"prompt_tokens":12`) {
		t.Errorf("expected prompt_tokens 12 (10+2), got: %s", outStr)
	}
}
