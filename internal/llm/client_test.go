package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/noel88/tsuzuri/internal/config"
)

func TestFakeClientRecordsRequest(t *testing.T) {
	f := &FakeClient{Reply: `{"ok":true}`}
	got, err := f.Complete(context.Background(), Request{
		System: "시스템", User: "사용자", ToolName: "emit",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != `{"ok":true}` {
		t.Errorf("Reply = %q", got)
	}
	if f.Got.System != "시스템" || f.Got.User != "사용자" {
		t.Errorf("요청이 기록되어야 한다: %+v", f.Got)
	}
	if f.Calls != 1 {
		t.Errorf("호출 횟수 = %d", f.Calls)
	}
}

func TestFakeClientReturnsError(t *testing.T) {
	want := errors.New("실패")
	f := &FakeClient{Err: want}
	if _, err := f.Complete(context.Background(), Request{}); !errors.Is(err, want) {
		t.Errorf("오류가 전달되어야 한다: %v", err)
	}
}

func TestNewAnthropicRequiresKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := NewAnthropic(config.Config{Model: "claude-opus-5"}); err == nil {
		t.Error("키가 없으면 오류여야 한다")
	}
}

func TestNewAnthropicAcceptsValidConfig(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	c, err := NewAnthropic(config.Config{Model: "claude-opus-5"})
	if err != nil {
		t.Fatalf("생성에 실패하면 안 된다: %v", err)
	}
	if c == nil {
		t.Error("클라이언트가 nil이면 안 된다")
	}
}

func TestClientSatisfiesInterface(t *testing.T) {
	// 상위 패키지가 인터페이스에만 의존하도록 고정한다.
	var _ Client = (*FakeClient)(nil)
	var _ Client = (*anthropicClient)(nil)
}

func TestIsEmptyInput(t *testing.T) {
	for _, s := range []string{"", "  ", "{}", " {} ", "null"} {
		if !isEmptyInput(s) {
			t.Errorf("%q는 빈 입력이다", s)
		}
	}
	for _, s := range []string{`{"a":1}`, `{"corrected":""}`} {
		if isEmptyInput(s) {
			t.Errorf("%q를 빈 입력으로 보면 안 된다", s)
		}
	}
}

// 스트림이 도중에 끊기면 SDK가 덜 받은 툴 입력을 "{}"로 되돌린다.
// 그것을 성공으로 넘기면 빈 첨삭이 영구 기록되고, 그 답안은 이미 첨삭받은
// 것으로 처리돼 다시는 요청되지 않는다.
func TestExtractJSONRejectsEmptyToolInput(t *testing.T) {
	var msg anthropic.Message
	raw := `{"stop_reason":"refusal","content":[{"type":"tool_use","id":"t1","name":"emit","input":{}}]}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	got, err := extractJSON(msg, "문항 수를 줄여 보세요")
	if err == nil {
		t.Fatalf("끊긴 응답을 성공으로 넘기면 안 된다: %q", got)
	}
	if !strings.Contains(err.Error(), "끊겼") {
		t.Errorf("무슨 일이 났는지 알려야 한다: %v", err)
	}
}

func TestExtractJSONAcceptsRealToolInput(t *testing.T) {
	var msg anthropic.Message
	raw := `{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"emit","input":{"echo":"ping"}}]}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	got, err := extractJSON(msg, "문항 수를 줄여 보세요")
	if err != nil {
		t.Fatalf("정상 응답인데 실패했다: %v", err)
	}
	if !strings.Contains(got, "ping") {
		t.Errorf("툴 입력을 그대로 돌려줘야 한다: %q", got)
	}
}

func TestExtractJSONReportsTruncation(t *testing.T) {
	var msg anthropic.Message
	raw := `{"stop_reason":"max_tokens","content":[{"type":"tool_use","id":"t1","name":"emit","input":{"a":1}}]}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if _, err := extractJSON(msg, "문항 수를 줄여 보세요"); err == nil || !strings.Contains(err.Error(), "문항 수") {
		t.Errorf("토큰 한도 절단을 알려야 한다: %v", err)
	}
}

// 긴 생성은 정상적으로도 몇 분 걸린다. 응답이 계속 오는 동안에는 끊지 않고,
// 멈춰 있을 때만 끊어야 한다.
func TestWatchStallKeepsWaitingWhileDataArrives(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	called := make(chan struct{})
	beat := make(chan struct{}, 1)
	go watchStall(ctx, beat, func() { close(called) }, 60*time.Millisecond, new(atomic.Bool))

	// 40ms마다 데이터가 오는 상황을 200ms 동안 이어간다.
	for i := 0; i < 5; i++ {
		time.Sleep(40 * time.Millisecond)
		beat <- struct{}{}
	}
	select {
	case <-called:
		t.Fatal("데이터가 계속 오는데 끊었다")
	default:
	}
}

func TestWatchStallCancelsWhenStreamGoesQuiet(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	called := make(chan struct{})
	go watchStall(ctx, make(chan struct{}), func() { close(called) }, 50*time.Millisecond, new(atomic.Bool))

	select {
	case <-called:
	case <-time.After(3 * time.Second):
		t.Fatal("멈춘 연결을 끊지 않았다")
	}
}

// Haiku는 thinking {type: adaptive}를 받지 않는다. 그대로 보내면 400으로
// 거절당해 온라인 기능이 통째로 막힌다.
func TestAdaptiveThinkingSkippedForHaiku(t *testing.T) {
	for _, m := range []string{"claude-haiku-4-5", "Claude-Haiku-4-5", "claude-haiku-4-5-20251001"} {
		if supportsAdaptiveThinking(m) {
			t.Errorf("%q에는 적응형 사고를 보내면 안 된다", m)
		}
	}
	for _, m := range []string{"claude-opus-5", "claude-sonnet-5", "claude-fable-5-1"} {
		if !supportsAdaptiveThinking(m) {
			t.Errorf("%q에는 적응형 사고를 보내야 한다", m)
		}
	}
}

func TestWatchStallMarksThatItCut(t *testing.T) {
	// 우리가 끊은 것이면 그렇게 말해야 한다. 표시가 없으면 사용자는
	// 「context canceled」를 보고 무엇을 바꿔야 할지 알 수 없다.
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var stalled atomic.Bool
	done := make(chan struct{})
	go watchStall(ctx, make(chan struct{}), func() { close(done) }, 30*time.Millisecond, &stalled)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("멈춘 연결을 끊지 않았다")
	}
	if !stalled.Load() {
		t.Error("끊었다는 표시를 남기지 않았다")
	}
}
