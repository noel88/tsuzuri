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

// Cursor는 초기화면에서 지금 가리키는 자리다.
//
// 화살표로 옮긴다. 번호를 쳐서 고르는 길은 그대로 두므로 커서가 없어도
// 앱을 쓸 수 있다 — 기기에서 화살표가 안 먹혀도 막히지 않는다.
type Cursor struct {
	Right bool // 자료실(오른쪽) 칸인지
	Index int
}

// menuEntry는 왼쪽 칸의 항목 하나다.
type menuEntry struct{ key, label string }

func leftEntries(st Status, choices []Choice) []menuEntry {
	review := "복습 드릴"
	if st.Due > 0 {
		review = fmt.Sprintf("복습 드릴 (%d)", st.Due)
	}
	// 「드릴 시작」은 자료실 맨 위 팩을 연다. 어느 팩인지 적어 두지 않으면
	// 누르기 전에는 알 수 없다 — 자료실이 팩마다 한 줄이 된 뒤로 그 줄이
	// 여럿이고, 무엇이 맨 위인지는 정렬 규칙을 알아야 짐작이 된다.
	start := "드릴 시작"
	if len(choices) > 0 {
		start = fmt.Sprintf("드릴 시작 (%s)", choices[0].Key)
	}
	return []menuEntry{
		{"1", start},
		{"2", "이어하기"},
		{"3", review},
		{"4", "첨삭 보기"},
		{"5", "진도"},
		{"6", "내보내기"},
		{"7", "설정"},
		{"8", "팩 받기"},
		{"9", "첨삭 받기"},
	}
}

// MenuKeyAt은 커서가 가리키는 항목의 번호를 돌려준다.
func MenuKeyAt(choices []Choice, st Status, c Cursor) (string, bool) {
	if c.Right {
		if c.Index < 0 || c.Index >= len(choices) {
			return "", false
		}
		return choices[c.Index].Key, true
	}
	left := leftEntries(st, choices)
	if c.Index < 0 || c.Index >= len(left) {
		return "", false
	}
	return left[c.Index].key, true
}

// MoveCursor는 화살표 한 번에 커서를 옮긴다.
//
// 끝에서 더 눌러도 넘어가지 않고 멈춘다. 아홉 줄짜리 목록에서 위로 계속
// 눌렀을 때 맨 아래로 돌아가면 지금 어디에 있는지 놓치게 된다.
func MoveCursor(c Cursor, k Key, choices []Choice, st Status) Cursor {
	n := len(leftEntries(st, choices))
	if c.Right {
		n = len(choices)
	}

	switch k {
	case KeyUp:
		if c.Index > 0 {
			c.Index--
		}
	case KeyDown:
		if c.Index < n-1 {
			c.Index++
		}
	case KeyRight:
		// 자료실이 비어 있으면 건너갈 곳이 없다.
		if !c.Right && len(choices) > 0 {
			c.Right = true
			c.Index = clamp(c.Index, len(choices))
		}
	case KeyLeft:
		if c.Right {
			c.Right = false
			c.Index = clamp(c.Index, len(leftEntries(st, choices)))
		}
	}
	return c
}

func clamp(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// ClampCursor는 목록이 줄어들어 커서가 밖으로 나간 것을 안으로 들인다.
// 팩을 지우면 자료실이 짧아진다.
func ClampCursor(c Cursor, choices []Choice, st Status) Cursor {
	if c.Right && len(choices) == 0 {
		c.Right = false
	}
	n := len(leftEntries(st, choices))
	if c.Right {
		n = len(choices)
	}
	c.Index = clamp(c.Index, n)
	return c
}

// RenderMenu는 초기화면이다. PC통신 게시판 양식을 따른다.
//
// 번호를 쳐서 이동하는 것이 이 양식의 본래 조작법이고, 화살표는 거기에
// 얹은 것이다. cur이 가리키는 줄에 표시를 단다.
func RenderMenu(choices []Choice, st Status, termW int, cur Cursor) string {
	w := BoxWidth(termW)

	var lines []string
	lines = append(lines, Banner("綴 / TSUZURI", "한-일 번역 작문 드릴", w)...)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("   팩 %d개 · 문제 %d · 첨삭 대기 %d건 · 받은 첨삭 %d건",
		len(choices), st.Total, st.QueueLen, st.Feedback))
	lines = append(lines, "")

	leftW := 28
	// 오른쪽 칼럼은 넓혀도 읽기 좋아지지 않는다. 화면이 넓으면 수치만
	// 저 멀리 오른쪽 끝으로 밀려나 항목과 떨어져 보인다.
	rightW := w - leftW - 6
	if rightW > 36 {
		rightW = 36
	}
	if rightW < 20 {
		rightW = 20
	}

	var left []string
	for i, e := range leftEntries(st, choices) {
		item := MenuItem(e.key, e.label, 22)
		item[0] = point(item[0], !cur.Right && cur.Index == i)
		left = append(left, item...)
	}

	var right []string
	right = append(right, Box("자 료 실")...)
	if len(choices) == 0 {
		// 기기 앞에 앉은 사람에게 파일 경로는 쓸모가 없다. 셸로 나가지
		// 않고 여기서 할 수 있는 일을 말한다.
		right = append(right, "  (팩이 없습니다. 8번으로 받으세요)")
	}
	for i, c := range choices {
		right = append(right, point(Entry(c.Key, c.Label, c.Value, rightW), cur.Right && cur.Index == i))
	}

	lines = append(lines, TwoCol(left, right, leftW, 4)...)
	lines = append(lines, "")
	lines = append(lines, CommandBarWith(
		fmt.Sprintf("첨삭 큐 %d건 · %s", st.QueueLen, connLabel(st.Online)),
		"주요명령(화살표·번호)  8·9는 네트워크 필요  종료(X)",
		"선택 >>", w)...)

	return Page(lines, termW, w)
}

// point는 줄 앞의 여백을 표시로 바꾼다.
//
// 폭이 변하지 않아야 오른쪽 칸이 흔들리지 않는다. 색이나 반전 대신 글자를
// 쓰는 것은 fbterm에서 그것이 어떻게 보일지 재 보지 않았기 때문이다.
func point(line string, on bool) string {
	if !on || !strings.HasPrefix(line, "  ") {
		return line
	}
	return "> " + line[2:]
}

// ParseMenuKey는 초기화면 입력을 해석한다.
// 번호를 고르면 그 키를, 종료면 빈 문자열과 false를 돌려준다.
//
// 번호를 다른 번호로 바꾸지 않는다. 예전에는 「1」을 첫 팩 번호로 바꿔
// 돌려줬는데, 그 변환이 입력을 읽는 자리에 숨어 있어서 화살표로 고른
// 길은 그것을 거치지 않았다 — 「드릴 시작」을 고르면 그런 번호가 없다고
// 답했다. 번호의 뜻은 그것을 처리하는 쪽이 안다.
func ParseMenuKey(s string) (key string, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "x", "q":
		return "", false
	}
	return s, true
}
