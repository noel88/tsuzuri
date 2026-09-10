package ui

import (
	"strings"
	"testing"
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

func TestBoxWidthCapsAt70(t *testing.T) {
	if got := BoxWidth(200); got != 70 {
		t.Errorf("BoxWidth(200) = %d, 기대 70", got)
	}
}

func TestBoxWidthUsesFullNarrowScreen(t *testing.T) {
	if got := BoxWidth(50); got != 50 {
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
