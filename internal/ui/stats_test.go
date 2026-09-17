package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/noel88/tsuzuri/internal/progress"
)

func TestRenderStatsShowsNoScore(t *testing.T) {
	// 스펙 §5.4. 진도는 「얼마나 했는지」이지 「얼마나 잘했는지」가 아니다.
	s := progress.Stats{
		Total: 40, Problems: 12, Days: 5, Streak: 3, Feedback: 30,
		Due: 4, Waiting: 2, Graduated: 6,
		TopMissing: []progress.Count{{Text: "つもりだ", N: 7}},
	}
	out := RenderStats(s, nil, nil, Status{}, 100)

	for _, want := range []string{"연속", "3일", "복습할 문제", "4개", "자주 빠뜨린 표현", "つもりだ", "7번"} {
		if !strings.Contains(out, want) {
			t.Errorf("진도 화면에 %q가 없다:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"점수", "정답률", "%"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("진도 화면에 점수를 뜻하는 %q가 있다:\n%s", unwanted, out)
		}
	}
}

func TestRenderStatsShowsNextReviewAsRemainingTime(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	cards := []progress.Card{
		{ProblemID: "p1", State: progress.StateScheduled, Due: now.AddDate(0, 0, 3)},
	}
	out := RenderStats(progress.Stats{}, cards, map[string]string{"p1": "비가 올 것 같다"}, Status{Now: now}, 100)

	if !strings.Contains(out, "비가 올 것 같다") {
		t.Errorf("다음 복습에 문제가 안 보인다:\n%s", out)
	}
	// 정확한 시각이 아니라 남은 시간으로 보여준다.
	if !strings.Contains(out, "3일 뒤") {
		t.Errorf("남은 시간이 안 보인다:\n%s", out)
	}
}

func TestRenderStatsFitsTerminal(t *testing.T) {
	s := progress.Stats{TopMissing: []progress.Count{{Text: strings.Repeat("길", 60), N: 1}}}
	for _, line := range strings.Split(RenderStats(s, nil, nil, Status{}, 128), "\n") {
		if Width(line) > 128 {
			t.Errorf("줄이 화면을 넘는다 (%d칸): %q", Width(line), line)
		}
	}
}
