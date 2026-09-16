package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

// stallTimeout은 스트림이 아무것도 보내지 않는 채 견딜 수 있는 시간이다.
//
// 포메라의 Wi-Fi는 불안정하다. 시한이 없으면 멈춘 연결에서 무한히
// 대기하게 되고, 사용자에게는 진행도, 오류도, 빠져나갈 길도 없다.
//
// 호출 전체에 시한을 걸면 안 된다. 50문항짜리 팩 생성은 정상적으로도
// 몇 분씩 걸리는데, 그때 잘라 버리면 토큰 값은 이미 나간 뒤에 결과만
// 잃는다. 그래서 "멈춰 있는 동안"만 잰다.
const stallTimeout = 3 * time.Minute

// maxCallTimeout은 아무리 길어도 이보다 오래 붙잡고 있지 않는다.
const maxCallTimeout = 30 * time.Minute

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
		api: anthropic.NewClient(
			option.WithAPIKey(c.ResolvedKey()),
			option.WithRequestTimeout(maxCallTimeout),
		),
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

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.User)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
	}
	// 적응형 사고를 지원하지 않는 모델에 보내면 400으로 거절당해
	// 온라인 기능이 통째로 막힌다.
	if supportsAdaptiveThinking(a.model) {
		adaptive := anthropic.ThinkingConfigAdaptiveParam{}
		params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive}
	}

	// 긴 출력이 HTTP 타임아웃에 걸리지 않도록 스트리밍으로 받아 누적한다.
	ctx, cancel := context.WithTimeout(ctx, maxCallTimeout)
	defer cancel()

	// 스트림이 멈춰 있는 동안만 시한을 잰다. 무언가 도착할 때마다 다시 센다.
	stalled, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	beat := make(chan struct{}, 1)
	go watchStall(stalled, beat, cancel, stallTimeout)

	stream := a.api.Messages.NewStreaming(ctx, params)
	var msg anthropic.Message
	for stream.Next() {
		select {
		case beat <- struct{}{}:
		default:
		}
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("응답 누적 실패: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", err
	}

	return extractJSON(msg)
}

// watchStall은 beat가 timeout 동안 오지 않으면 호출을 끊는다.
func watchStall(ctx context.Context, beat <-chan struct{}, cancel context.CancelFunc, timeout time.Duration) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-beat:
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			t.Reset(timeout)
		case <-t.C:
			cancel()
			return
		}
	}
}

// extractJSON은 응답에서 구조화된 결과를 꺼낸다.
// 툴 호출이 정상 경로이고, 모델이 텍스트로 답한 경우를 대비책으로 둔다.
func extractJSON(msg anthropic.Message) (string, error) {
	for _, block := range msg.Content {
		v, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}
		if truncated(msg) {
			return "", fmt.Errorf(
				"응답이 토큰 한도에서 잘렸습니다. 팩 크기를 줄여 보세요 (설정 5번)")
		}
		raw := v.JSON.Input.Raw()
		// 스트림이 도중에 끊기면 SDK가 덜 받은 입력을 "{}"로 되돌린다.
		// 그대로 성공으로 넘기면 빈 첨삭이 영구 기록되고, 그 답안은 이미
		// 첨삭받은 것으로 처리돼 다시는 요청되지 않는다.
		if isEmptyInput(raw) {
			return "", fmt.Errorf(
				"응답이 도중에 끊겼습니다 (stop_reason=%q). 다시 시도하세요", msg.StopReason)
		}
		return raw, nil
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

// supportsAdaptiveThinking은 그 모델이 thinking {type: adaptive}를 받는지 본다.
//
// Haiku 계열은 받지 않는다. 설정에서 모델 이름을 자유롭게 적을 수 있으므로,
// 모르는 이름은 보내 보는 쪽(적응형 사용)을 기본으로 두고 Haiku만 뺀다.
func supportsAdaptiveThinking(model string) bool {
	return !strings.Contains(strings.ToLower(model), "haiku")
}

// isEmptyInput은 툴 입력이 비어 있는지 본다.
func isEmptyInput(raw string) bool {
	t := strings.TrimSpace(raw)
	return t == "" || t == "{}" || t == "null"
}

// truncated는 응답이 토큰 한도에서 잘렸는지 본다.
//
// 잘린 툴 호출은 반쪽짜리 JSON을 남기므로, 그대로 두면 상위에서
// "unexpected end of JSON input"만 보게 된다 — 이미 전액 청구된 뒤에.
func truncated(msg anthropic.Message) bool {
	return msg.StopReason == anthropic.StopReasonMaxTokens
}
