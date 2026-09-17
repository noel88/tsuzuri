package ui

import "strings"

// Action은 결과 화면에서 고를 수 있는 것 하나다.
type Action struct {
	Cmd   Command
	Key   string // 눌러도 되는 글자
	Label string
}

// ResultActions는 결과 화면의 선택지다. 순서가 곧 좌우 이동 순서다.
//
// 「다음」이 맨 앞이다. 대부분은 다음 문제로 넘어가고, 그때는 화살표를
// 건드릴 것 없이 Enter만 누르면 된다.
var ResultActions = []Action{
	{CmdNext, "", "다음"},
	{CmdPriority, "F", "첨삭 먼저"},
	{CmdRetry, "R", "다시"},
	{CmdMark, "B", "복습에 넣기"},
	{CmdMenu, "M", "메뉴"},
	{CmdQuit, "X", "종료"},
}

// AnswerActions는 답안 화면에서 빈 줄로 Enter를 눌렀을 때의 선택지다.
//
// 답을 치는 동안에는 이 줄이 뜨지 않는다. 그 화면은 터미널의 보통 입력
// 경로를 그대로 써야 하고 — 그래야 입력기가 동작하고 친 글자가 화면에
// 보인다 — 키를 하나씩 받으려면 그 에코를 우리가 꺼야 하기 때문이다.
// 아무것도 치고 있지 않은 이 순간에만 키를 하나씩 받는다.
var AnswerActions = []Action{
	{CmdStay, "", "계속 쓰기"},
	{CmdEdit, ":e", "긴 답"},
	{CmdNext, "", "건너뛰기"},
	{CmdMenu, "M", "메뉴"},
	{CmdQuit, "X", "종료"},
}

// ActionAt은 고른 자리의 명령을 돌려준다.
func ActionAt(as []Action, sel int) Command {
	if sel < 0 || sel >= len(as) {
		return CmdNext
	}
	return as[sel].Cmd
}

// MoveAction은 좌우 화살표로 선택을 옮긴다. 끝에서 멈춘다.
func MoveAction(sel int, k Key, n int) int {
	switch k {
	case KeyLeft:
		if sel > 0 {
			sel--
		}
	case KeyRight:
		if sel < n-1 {
			sel++
		}
	}
	return sel
}

// ActionBar는 선택지를 한 줄로 그린다. 고른 것에 괄호를 씌운다.
//
//	[다음]   첨삭 먼저(F)   다시(R)   복습에 넣기(B)   메뉴(M)   종료(X)
//
// 글자 키를 함께 적는 것은 화살표가 안 먹히는 자리에서도 쓸 수 있어야
// 하기 때문이다. 입력기가 켜져 있으면 글자 키가 조합으로 먹히는데,
// 화살표는 입력기를 지나가므로 둘이 서로를 메운다.
func ActionBar(as []Action, sel int) string {
	var b strings.Builder
	for i, a := range as {
		b.WriteString("  ")
		label := a.Label
		if a.Key != "" {
			label += "(" + a.Key + ")"
		}
		if i == sel {
			b.WriteString("[" + label + "]")
		} else {
			b.WriteString(" " + label + " ")
		}
	}
	return b.String()
}
