package llm

import (
	"context"
	"errors"
	"testing"

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
