package ui

import (
	"strings"
	"testing"

	"github.com/noel88/tsuzuri/internal/config"
)

func TestRuleCentersTitle(t *testing.T) {
	got := Rule(" p047  일상 ", 40)
	if Width(got) != 40 {
		t.Fatalf("가로줄 폭 = %d, 기대 40 (%q)", Width(got), got)
	}
	if !strings.Contains(got, " p047  일상 ") {
		t.Errorf("제목이 들어 있어야 한다: %q", got)
	}
	// 좌우 채움 길이 차이가 1 이하여야 가운데다.
	// 바이트 인덱스가 아니라 채움 문자 개수를 세야 한다.
	left := countLeading(got, '─')
	right := countTrailing(got, '─')
	if left-right > 1 || right-left > 1 {
		t.Errorf("좌우 여백이 %d / %d — 가운데가 아니다", left, right)
	}
}

func TestRuleWithCJKTitleKeepsWidth(t *testing.T) {
	// 한글이 두 칸을 차지하므로 rune 개수로 계산하면 폭이 어긋난다.
	for _, title := range []string{" 일상 ", " p047 ", " 큐 12건 · 오프라인 "} {
		if got := Rule(title, 50); Width(got) != 50 {
			t.Errorf("Rule(%q) 폭 = %d, 기대 50", title, Width(got))
		}
	}
}

func TestRuleTruncatesOverlongTitle(t *testing.T) {
	got := Rule(" 아주아주아주아주 긴 제목입니다 ", 10)
	if Width(got) > 10 {
		t.Errorf("폭 %d가 10을 넘는다: %q", Width(got), got)
	}
}

func TestBoxWidthLeavesSafetyMargin(t *testing.T) {
	// 「↔」처럼 폭이 애매한 문자를 터미널이 두 칸으로 그리면 한 칸만 넘쳐도
	// 줄이 접혀 상자가 통째로 어긋난다. 실기에서 제목 줄이 깨진 적이 있다.
	for _, termW := range []int{128, 200} {
		if got, want := BoxWidth(termW), termW-safetyMargin; got != want {
			t.Errorf("BoxWidth(%d) = %d, 기대 %d", termW, got, want)
		}
	}
}

func TestBoxWidthNeverExceedsScreen(t *testing.T) {
	for termW := 10; termW <= 200; termW++ {
		if got := BoxWidth(termW); got > termW {
			t.Errorf("BoxWidth(%d) = %d, 화면보다 넓다", termW, got)
		}
	}
}

func TestBoxWidthUsesNarrowScreenFully(t *testing.T) {
	// 좁은 화면에서까지 6칸을 떼면 남는 게 없다. 거의 다 쓴다.
	if got := BoxWidth(50); got < 44 {
		t.Errorf("좁은 화면에서는 꽉 써야 한다: %d", got)
	}
}

func TestCenterIndentsBlockButKeepsLeftAlignment(t *testing.T) {
	lines := []string{"나:   AAA", "참조: BBB"}
	got := Center(lines, 90, 70)

	m := Margin(90, 70)
	if len(m) != 10 {
		t.Fatalf("여백 = %d칸, 기대 10", len(m))
	}
	// 두 줄의 들여쓰기가 같아야 문장 시작 위치가 맞는다.
	i0 := len(got[0]) - len(strings.TrimLeft(got[0], " "))
	i1 := len(got[1]) - len(strings.TrimLeft(got[1], " "))
	if i0 != i1 {
		t.Errorf("나/참조의 들여쓰기가 달라 비교가 안 된다: %d vs %d", i0, i1)
	}
}

func TestCenterLeavesBlankLinesEmpty(t *testing.T) {
	got := Center([]string{"", "본문"}, 90, 70)
	if got[0] != "" {
		t.Errorf("빈 줄에 공백을 채우면 안 된다: %q", got[0])
	}
}

func TestCenterNoMarginWhenBlockFillsScreen(t *testing.T) {
	got := Center([]string{"본문"}, 70, 70)
	if got[0] != "본문" {
		t.Errorf("여백이 없어야 한다: %q", got[0])
	}
}

func countLeading(s string, r rune) int {
	n := 0
	for _, c := range s {
		if c != r {
			break
		}
		n++
	}
	return n
}

func countTrailing(s string, r rune) int {
	rs := []rune(s)
	n := 0
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] != r {
			break
		}
		n++
	}
	return n
}

func TestScreensEndOnThePromptLine(t *testing.T) {
	// 화면 끝에 개행이 붙으면 커서가 다음 줄 맨 왼쪽으로 내려가, 사용자가
	// 치는 답이 프롬프트 옆이 아니라 그 아래 여백 밖에 찍힌다.
	// 스크린샷을 찍어 보고서야 드러났다.
	screens := map[string]string{
		"출제":   RenderProblem(sampleProblem(), sampleStatus(), 92),
		"결과":   RenderResult(sampleProblem(), "답안", noisyAnalysis(), sampleStatus(), 92, 0),
		"초기화면": RenderMenu([]Choice{{Key: "41", Label: "ko2ja  N3 일상", Value: "2"}}, Status{}, 92, Cursor{}),
		"설정":   RenderSetup(config.Default(), Status{}, 92),
	}
	for name, out := range screens {
		if !strings.HasSuffix(out, ">> ") {
			tail := out
			if len(tail) > 30 {
				tail = tail[len(tail)-30:]
			}
			t.Errorf("%s 화면이 프롬프트(\">> \")로 끝나야 한다. 끝부분: %q", name, tail)
		}
	}
}
