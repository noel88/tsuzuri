package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/noel88/tsuzuri/internal/config"
)

// Request는 한 번의 생성 요청이다.
//
// 툴을 하나 정의해 넘기지만 그 툴은 실행되지 않는다. 모델이 툴을 호출하게
// 하고 그 **입력 JSON**을 결과로 받아가는, 구조화된 출력을 얻는 수단이다.
type Request struct {
	System    string
	User      string
	ToolName  string
	ToolDesc  string
	Schema    map[string]any // JSON Schema의 properties
	Required  []string
	MaxTokens int64
}

// Client는 네트워크를 아는 유일한 인터페이스다.
// 반환값은 툴 입력의 원시 JSON 문자열이며, 파싱은 호출한 쪽이 한다.
type Client interface {
	Complete(ctx context.Context, req Request) (string, error)
}

// FakeClient는 테스트용이다.
// 실제 호출은 비용이 들므로 상위 패키지의 단위 테스트는 전부 이것을 쓴다.
type FakeClient struct {
	Reply string
	Err   error
	Got   Request
	Calls int
}

func (f *FakeClient) Complete(_ context.Context, req Request) (string, error) {
	f.Got = req
	f.Calls++
	if f.Err != nil {
		return "", f.Err
	}
	return f.Reply, nil
}

type anthropicClient struct {
	api   anthropic.Client
	model string
}

// NewAnthropic은 실제 API 클라이언트를 만든다.
func NewAnthropic(c config.Config) (Client, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &anthropicClient{
		api:   anthropic.NewClient(option.WithAPIKey(c.ResolvedKey())),
		model: c.Model,
	}, nil
}

func (a *anthropicClient) Complete(ctx context.Context, req Request) (string, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 16000
	}

	tool := anthropic.ToolParam{
		Name:        req.ToolName,
		Description: anthropic.String(req.ToolDesc),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: req.Schema,
			ExtraFields: map[string]any{
				"additionalProperties": false,
				"required":             req.Required,
			},
		},
		Strict: anthropic.Bool(true),
	}

	adaptive := anthropic.ThinkingConfigAdaptiveParam{}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxTokens,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive},
		System: []anthropic.TextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.User)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
	}

	// 긴 출력이 HTTP 타임아웃에 걸리지 않도록 스트리밍으로 받아 누적한다.
	stream := a.api.Messages.NewStreaming(ctx, params)
	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("응답 누적 실패: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", err
	}

	return extractJSON(msg)
}

// extractJSON은 응답에서 구조화된 결과를 꺼낸다.
// 툴 호출이 정상 경로이고, 모델이 텍스트로 답한 경우를 대비책으로 둔다.
func extractJSON(msg anthropic.Message) (string, error) {
	for _, block := range msg.Content {
		if v, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			return v.JSON.Input.Raw(), nil
		}
	}
	for _, block := range msg.Content {
		if v, ok := block.AsAny().(anthropic.TextBlock); ok && json.Valid([]byte(v.Text)) {
			return v.Text, nil
		}
	}
	if msg.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("요청이 거부되었습니다: %s", msg.StopDetails.Explanation)
	}
	return "", fmt.Errorf("구조화된 응답을 받지 못했습니다 (stop_reason=%q)", msg.StopReason)
}
