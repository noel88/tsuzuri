package ui

import (
	"os"
	"strconv"
	"strings"
)

// 터미널에서 두 칸을 차지하는 문자 범위 (East Asian Wide / Fullwidth).
//
// 한글·한자·가나가 여기 들어간다. 이걸 고려하지 않고 문자 개수로
// 줄 길이를 계산하면 CJK가 섞인 순간 테두리가 어긋난다.
var wideRanges = [][2]rune{
	{0x1100, 0x115F},   // 한글 자모
	{0x2E80, 0x303E},   // CJK 부수, 강희 부수, CJK 기호 (「」 포함)
	{0x3041, 0x33FF},   // 히라가나, 가타카나, 한글 호환 자모, 원문자
	{0x3400, 0x4DBF},   // CJK 확장 A
	{0x4E00, 0x9FFF},   // CJK 통합 한자
	{0xA000, 0xA4CF},   // 이 문자
	{0xAC00, 0xD7A3},   // 한글 음절
	{0xF900, 0xFAFF},   // CJK 호환 한자
	{0xFE30, 0xFE6F},   // CJK 호환 형태
	{0xFF00, 0xFF60},   // 전각 형태
	{0xFFE0, 0xFFE6},   // 전각 기호
	{0x20000, 0x2FFFD}, // CJK 확장 B~
	{0x30000, 0x3FFFD},
}

// RuneWidth는 문자 하나가 터미널에서 차지하는 칸 수다.
func RuneWidth(r rune) int {
	for _, rg := range wideRanges {
		if r >= rg[0] && r <= rg[1] {
			return 2
		}
	}
	return 1
}

// Width는 문자열의 터미널 표시폭이다. len()이나 rune 개수가 아니다.
func Width(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// Truncate는 표시폭이 max를 넘지 않도록 문자열을 자른다.
// 문자 중간에서 자르지 않는다.
func Truncate(s string, max int) string {
	if Width(s) <= max {
		return s
	}
	w := 0
	var b strings.Builder
	for _, r := range s {
		rw := RuneWidth(r)
		if w+rw > max {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

// TermWidth는 터미널 폭을 구한다.
//
// 포메라 화면은 1024x600이고 fbterm 폰트 크기에 따라 칸 수가 달라진다.
// COLUMNS를 읽되, 없거나 이상하면 보수적인 기본값을 쓴다.
func TermWidth() int {
	const fallback = 76
	v := os.Getenv("COLUMNS")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 40 || n > 400 {
		return fallback
	}
	return n
}
