package llm

import (
	"context"
	"os"
	"testing"

	"github.com/noel88/tsuzuri/internal/config"
)

// TestLiveComplete는 실제 API를 호출한다. **호출마다 비용이 든다.**
// 기본적으로 돌지 않으며 명시적으로 켤 때만 실행된다:
//
//	TSUZURI_LIVE=1 go test ./internal/llm/ -run TestLiveComplete -v
func TestLiveComplete(t *testing.T) {
	if os.Getenv("TSUZURI_LIVE") != "1" {
		t.Skip("실제 API를 호출한다. TSUZURI_LIVE=1 로 켠다")
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY가 없다")
	}

	if err := (Preflight{}).Check(context.Background()); err != nil {
		t.Fatalf("사전 점검 실패: %v", err)
	}

	c, err := NewAnthropic(config.Config{Model: "claude-opus-5"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.Complete(context.Background(), Request{
		System:   "테스트입니다. 도구로 짧게 답하세요.",
		User:     "ping이라는 값을 echo 필드에 넣어 제출하세요.",
		ToolName: "emit",
		ToolDesc: "결과를 제출합니다.",
		Schema: map[string]any{
			"echo": map[string]any{"type": "string"},
		},
		Required:  []string{"echo"},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	t.Logf("응답: %s", raw)
	if raw == "" {
		t.Error("빈 응답")
	}
}
