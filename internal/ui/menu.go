package ui

import (
	"fmt"
	"strings"
)

// Choice는 초기화면의 자료실 항목 하나다. 번호를 쳐서 고른다.
type Choice struct {
	Key   string // 사용자가 입력할 번호 ("41")
	Label string // "ko2ja  N3 일상"
	Value string // 오른쪽 끝 수치 ("47")
}

// RenderMenu는 초기화면이다. PC통신 게시판 양식을 따른다.
//
// 화살표 탐색이 아니라 번호 입력으로 이동한다 — raw mode를 쓰지 않으므로
// 어차피 타이핑이고, 그것이 이 양식의 원래 조작법이기도 하다.
func RenderMenu(choices []Choice, st Status, termW int) string {
	w := BoxWidth(termW)

	var lines []string
	lines = append(lines, Banner("綴 / T·S·U·Z·U·R·I", "한↔일 번역 작문 드릴", w)...)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("   팩 %d개 · 문제 %d · 첨삭 대기 %d건 · 받은 첨삭 %d건",
		len(choices), st.Total, st.QueueLen, st.Feedback))
	lines = append(lines, "")

	leftW := 28
	rightW := w - leftW - 6
	if rightW < 20 {
		rightW = 20
	}

	var left []string
	left = append(left, MenuItem("1", "드릴 시작", 22)...)
	left = append(left, MenuItem("2", "복습", 22)...)
	left = append(left, MenuItem("3", "내보내기", 22)...)
	left = append(left, MenuItem("4", "설정", 22)...)
	left = append(left, MenuItem("5", "팩 받기", 22)...)
	left = append(left, MenuItem("6", "첨삭 받기", 22)...)

	var right []string
	right = append(right, Box("자 료 실")...)
	if len(choices) == 0 {
		right = append(right, "  (팩이 없습니다. packs/ 에 넣으세요)")
	}
	for _, c := range choices {
		right = append(right, Entry(c.Key, c.Label, c.Value, rightW))
	}

	lines = append(lines, TwoCol(left, right, leftW, 4)...)
	lines = append(lines, "")
	lines = append(lines, CommandBarWith(
		fmt.Sprintf("첨삭 큐 %d건 · %s", st.QueueLen, connLabel(st.Online)),
		"주요명령(이동 번호)  5·6은 네트워크 필요  종료(X)",
		"선택 >>", w)...)

	return Join(Center(lines, termW, w))
}

// ParseMenuKey는 초기화면 입력을 해석한다.
// 번호를 고르면 그 키를, 종료면 빈 문자열과 false를 돌려준다.
func ParseMenuKey(s string) (key string, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "x", "q":
		return "", false
	case "1":
		// 「드릴 시작」은 첫 팩을 뜻한다.
		return "41", true
	}
	return s, true
}
