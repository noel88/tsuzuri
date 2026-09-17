package ui

import (
	"os"
	"strings"
	"testing"
)

func TestWidthCountsCJKAsTwo(t *testing.T) {
	cases := map[string]int{
		"":      0,
		"abc":   3,
		"일상":    4, // 한글 2자 = 4칸
		"初めて":   6, // 한자1 + 가나2 = 6칸
		"p047":  4,
		"「静かく」": 10, // 괄호도 전각이다
		"a일b":   4,
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

func TestTermWidthIsUsableWithoutATerminal(t *testing.T) {
	// go test는 stdout이 터미널이 아니므로 ioctl이 실패한다.
	// 그래도 쓸 만한 값이 나와야 한다.
	os.Unsetenv("COLUMNS")
	if got := TermWidth(); !sane(got) {
		t.Errorf("TermWidth = %d — 40~400 범위여야 한다", got)
	}
}

func TestTermWidthFallsBackToColumnsWhenNoTTY(t *testing.T) {
	// ioctl이 실패할 때의 차선책. COLUMNS는 셸이 export하지 않으면
	// 자식에게 전달되지 않으므로 이것에 의존할 수는 없다.
	t.Setenv("COLUMNS", "100")
	if got := TermWidth(); got != 100 {
		t.Errorf("TermWidth = %d, 기대 100", got)
	}
}

func TestTermWidthRejectsGarbage(t *testing.T) {
	for _, v := range []string{"", "abc", "5", "9999"} {
		t.Setenv("COLUMNS", v)
		if got := TermWidth(); got != fallbackWidth {
			t.Errorf("COLUMNS=%q일 때 TermWidth = %d, 기대 %d", v, got, fallbackWidth)
		}
	}
}

func TestTruncateMarkShowsThatItCut(t *testing.T) {
	// 잘린 표시가 없으면 문장이 이상하게 끝난 것처럼 읽힌다.
	got := TruncateMark("어제 처음 간 카페가 생각보다 조용해서 오래 앉아 있었다.", 20)
	if !strings.HasSuffix(got, "..") {
		t.Errorf("잘렸는데 표시가 없다: %q", got)
	}
	if Width(got) > 20 {
		t.Errorf("폭 = %d, 20을 넘으면 안 된다: %q", Width(got), got)
	}
	// 안 잘리면 그대로 둔다.
	if got := TruncateMark("짧다", 20); got != "짧다" {
		t.Errorf("멀쩡한 문자열을 건드렸다: %q", got)
	}
}
