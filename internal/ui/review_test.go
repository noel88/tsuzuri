package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/noel88/tsuzuri/internal/store"
	tsync "github.com/noel88/tsuzuri/internal/sync"
)

func TestRenderFeedbackShowsEverything(t *testing.T) {
	p := sampleProblem()
	a := store.Attempt{ID: "a001", PackID: p.ID, Answer: "昨日カフェは静かくて。"}
	f := tsync.Feedback{
		AttemptID: "a001",
		At:        time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC),
		Corrected: "昨日カフェは静かで。",
		Notes: []tsync.Note{
			{Span: "静かくて", Why: "な형용사는 「静かで」", Level: tsync.LevelError},
			{Span: "ずっと", Why: "「長く」가 더 가깝다", Level: tsync.LevelNuance},
		},
		Overall: "활용 오류 하나를 빼면 의미는 전달됩니다.",
	}
	out := RenderFeedback(p, a, f, Status{}, 92)

	for _, want := range []string{"静かくて", "静かで", "な형용사", "長く", "의미는 전달", "참조:"} {
		if !strings.Contains(out, want) {
			t.Errorf("출력에 %q가 없다:\n%s", want, out)
		}
	}
}

func TestRenderFeedbackDistinguishesErrorFromNuance(t *testing.T) {
	p := sampleProblem()
	a := store.Attempt{ID: "a001", PackID: p.ID, Answer: "답안"}
	f := tsync.Feedback{
		AttemptID: "a001",
		Notes: []tsync.Note{
			{Span: "B", Why: "더 낫다", Level: tsync.LevelNuance},
			{Span: "A", Why: "틀렸다", Level: tsync.LevelError},
		},
	}
	out := RenderFeedback(p, a, f, Status{}, 92)

	ai := strings.Index(out, "틀렸다")
	bi := strings.Index(out, "더 낫다")
	if ai < 0 || bi < 0 {
		t.Fatalf("두 지적이 모두 보여야 한다:\n%s", out)
	}
	if ai > bi {
		t.Error("입력 순서와 무관하게 error가 nuance보다 먼저 나와야 한다")
	}
	if !strings.Contains(out, "✗ 「A」") {
		t.Errorf("오류는 ✗로 표시해야 한다:\n%s", out)
	}
	if !strings.Contains(out, "· 「B」") {
		t.Errorf("뉘앙스는 ·로 표시해야 한다 — 오류처럼 보이면 안 된다:\n%s", out)
	}
}

func TestRenderFeedbackOmitsEmptyParts(t *testing.T) {
	p := sampleProblem()
	a := store.Attempt{ID: "a001", PackID: p.ID, Answer: "답안"}
	out := RenderFeedback(p, a, tsync.Feedback{AttemptID: "a001"}, Status{}, 92)

	if strings.Contains(out, "첨삭:") {
		t.Errorf("corrected가 없으면 그 줄이 없어야 한다:\n%s", out)
	}
	if strings.Contains(out, "✗") || strings.Contains(out, "·") {
		t.Errorf("지적이 없으면 그 블록이 없어야 한다:\n%s", out)
	}
}

func TestRenderFeedbackFitsTerminal(t *testing.T) {
	p := sampleProblem()
	a := store.Attempt{ID: "a001", PackID: p.ID, Answer: strings.Repeat("あ", 80)}
	f := tsync.Feedback{
		AttemptID: "a001",
		Corrected: strings.Repeat("い", 80),
		Notes:     []tsync.Note{{Span: "あ", Why: strings.Repeat("う", 60), Level: tsync.LevelError}},
		Overall:   strings.Repeat("え", 80),
	}
	for _, termW := range []int{60, 76, 92} {
		out := RenderFeedback(p, a, f, Status{}, termW)
		for _, line := range strings.Split(out, "\n") {
			if Width(line) > termW {
				t.Errorf("폭 %d에서 넘친다 (%d칸): %q", termW, Width(line), line)
			}
		}
	}
}
