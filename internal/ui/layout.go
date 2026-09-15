package ui

import "strings"

// maxBoxWidth는 본문 블록의 최대 폭이다.
// 이보다 넓어지면 한 줄이 너무 길어 읽기 나빠진다.
const maxBoxWidth = 70

// BoxWidth는 터미널 폭에 맞는 본문 블록 폭을 고른다.
// 좁은 화면에서는 여백을 포기하고 화면을 꽉 쓴다.
func BoxWidth(termW int) int {
	if termW < maxBoxWidth+4 {
		return termW
	}
	return maxBoxWidth
}

// Margin은 블록을 화면 가운데 두기 위한 왼쪽 여백이다.
func Margin(termW, boxW int) string {
	d := termW - boxW
	if d <= 0 {
		return ""
	}
	return strings.Repeat(" ", d/2)
}

// Rule은 제목을 가운데 둔 가로줄이다.
//
//	───────── p047  ko2ja  N3  일상 ─────────
//
// 제목이 블록보다 길면 잘라낸다.
func Rule(title string, w int) string {
	title = Truncate(title, w)
	fill := w - Width(title)
	if fill <= 0 {
		return title
	}
	left := fill / 2
	return strings.Repeat("─", left) + title + strings.Repeat("─", fill-left)
}

// Center는 블록 전체를 화면 가운데로 민다.
// 각 줄의 내용은 블록 안에서 왼쪽 정렬을 유지한다 —
// 「나」와 「참조」 문장의 시작 위치가 맞아야 비교가 되기 때문이다.
func Center(lines []string, termW, boxW int) []string {
	m := Margin(termW, boxW)
	out := make([]string, len(lines))
	for i, l := range lines {
		if l == "" {
			out[i] = ""
			continue
		}
		out[i] = m + l
	}
	return out
}

// Page는 화면 하나를 가운데 정렬해 출력할 문자열로 만든다.
//
// 마지막 줄은 입력 프롬프트(「답 >> 」「선택 >> 」)이므로 뒤에 개행을 붙이지
// 않는다. 붙이면 커서가 다음 줄 맨 왼쪽으로 내려가, 사용자가 치는 글자가
// 프롬프트 옆이 아니라 그 아래 가운데 여백 밖에 찍힌다.
func Page(lines []string, termW, boxW int) string {
	return strings.TrimSuffix(Join(Center(lines, termW, boxW)), "\n")
}

// Join은 줄들을 하나의 문자열로 합친다. 마지막 줄에도 개행이 붙는다.
func Join(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}
