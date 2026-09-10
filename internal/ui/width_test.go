package ui

import (
	"os"
	"testing"
)

func TestWidthCountsCJKAsTwo(t *testing.T) {
	cases := map[string]int{
		"":       0,
		"abc":    3,
		"일상":     4,  // 한글 2자 = 4칸
		"初めて":    6,  // 한자1 + 가나2 = 6칸
		"p047":   4,
		"「静かく」":  10, // 괄호도 전각이다
		"a일b":    4,
	}
	for in, want := range cases {
		if got := Width(in); got != want {
			t.Errorf("Width(%q) = %d, 기대 %d", in, got, want)
		}
	}
}

func TestWidthDiffersFromRuneCount(t *testing.T) {
	// 이 테스트가 존재하는 이유: rune 개수로 계산하면 테두리가 어긋난다.
	s := "어제 카페"
	if Width(s) == len([]rune(s)) {
		t.Errorf("CJK 문자열의 표시폭은 문자 개수와 달라야 한다: Width=%d runes=%d",
			Width(s), len([]rune(s)))
	}
}

func TestTruncateDoesNotSplitRunes(t *testing.T) {
	got := Truncate("初めて行った", 5)
	// 5칸 안에는 2칸짜리 두 개(4칸)까지만 들어간다.
	if Width(got) > 5 {
		t.Errorf("Truncate 결과가 5칸을 넘는다: %q (폭 %d)", got, Width(got))
	}
	if got != "初め" {
		t.Errorf("Truncate = %q, 기대 \"初め\"", got)
	}
}

func TestTruncateLeavesShortStrings(t *testing.T) {
	if got := Truncate("abc", 10); got != "abc" {
		t.Errorf("Truncate = %q", got)
	}
}

func TestTermWidthReadsColumns(t *testing.T) {
	t.Setenv("COLUMNS", "100")
	if got := TermWidth(); got != 100 {
		t.Errorf("TermWidth = %d, 기대 100", got)
	}
}

func TestTermWidthFallsBackOnGarbage(t *testing.T) {
	for _, v := range []string{"", "abc", "5", "9999"} {
		os.Setenv("COLUMNS", v)
		if got := TermWidth(); got != 76 {
			t.Errorf("COLUMNS=%q일 때 TermWidth = %d, 기대 76", v, got)
		}
	}
	os.Unsetenv("COLUMNS")
}
