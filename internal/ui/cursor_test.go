package ui

import (
	"strings"
	"testing"
)

func packs(n int) []Choice {
	var out []Choice
	for i := 0; i < n; i++ {
		out = append(out, Choice{Key: "4" + string(rune('1'+i)), Label: "pack", Value: "5"})
	}
	return out
}

func TestMoveCursorStopsAtBothEnds(t *testing.T) {
	// 끝에서 되감기면 지금 어디에 있는지 놓친다.
	st := Status{}
	c := Cursor{}
	for i := 0; i < 5; i++ {
		c = MoveCursor(c, KeyUp, nil, st)
	}
	if c.Index != 0 {
		t.Errorf("맨 위에서 위로 갔다: %+v", c)
	}
	for i := 0; i < 50; i++ {
		c = MoveCursor(c, KeyDown, nil, st)
	}
	if got, want := c.Index, len(leftEntries(st, nil))-1; got != want {
		t.Errorf("맨 아래 = %d, 기대 %d", got, want)
	}
}

func TestMoveCursorCrossesColumns(t *testing.T) {
	st := Status{}
	ch := packs(2)

	c := MoveCursor(Cursor{Index: 5}, KeyRight, ch, st)
	if !c.Right {
		t.Fatal("오른쪽으로 못 갔다")
	}
	// 왼쪽 여섯 번째에서 건너갔지만 자료실은 두 줄뿐이다.
	if c.Index != 1 {
		t.Errorf("index = %d, 자료실 마지막 줄이어야 한다", c.Index)
	}

	back := MoveCursor(c, KeyLeft, ch, st)
	if back.Right {
		t.Error("왼쪽으로 못 돌아왔다")
	}
}

func TestMoveCursorStaysWhenArchiveEmpty(t *testing.T) {
	// 팩이 없으면 건너갈 곳이 없다. 넘어가면 아무것도 못 고르는 자리에 선다.
	c := MoveCursor(Cursor{Index: 2}, KeyRight, nil, Status{})
	if c.Right {
		t.Errorf("빈 자료실로 건너갔다: %+v", c)
	}
}

func TestMenuKeyAtMatchesWhatIsDrawn(t *testing.T) {
	st := Status{}
	ch := packs(2)

	// 왼쪽 세 번째는 복습 드릴이다.
	if got, ok := MenuKeyAt(ch, st, Cursor{Index: 2}); !ok || got != "3" {
		t.Errorf("MenuKeyAt = %q, %v", got, ok)
	}
	if got, ok := MenuKeyAt(ch, st, Cursor{Right: true, Index: 1}); !ok || got != ch[1].Key {
		t.Errorf("자료실 두 번째 = %q, %v", got, ok)
	}
	if _, ok := MenuKeyAt(ch, st, Cursor{Right: true, Index: 9}); ok {
		t.Error("없는 자리를 골라 줬다")
	}
}

func TestClampCursorAfterPacksDisappear(t *testing.T) {
	// 팩을 지우면 자료실이 짧아진다. 커서가 밖에 남으면 Enter가 안 먹는다.
	c := ClampCursor(Cursor{Right: true, Index: 4}, nil, Status{})
	if c.Right {
		t.Errorf("자료실이 비었는데 오른쪽에 남았다: %+v", c)
	}
	if _, ok := MenuKeyAt(nil, Status{}, c); !ok {
		t.Error("들여놓은 자리인데 고를 수 없다")
	}
}

func TestDrillStartSaysWhichPackItOpens(t *testing.T) {
	// 자료실이 여러 줄이면 「맨 위」가 어느 팩인지 눌러 보기 전에는 모른다.
	out := RenderMenu(packs(3), Status{}, 100, Cursor{})
	if !strings.Contains(out, "1. 드릴 시작 (41)") {
		t.Errorf("어느 팩을 여는지 안 적혀 있다:\n%s", out)
	}
	// 팩이 없으면 붙일 번호도 없다.
	if out := RenderMenu(nil, Status{}, 100, Cursor{}); strings.Contains(out, "드릴 시작 (") {
		t.Errorf("팩이 없는데 번호가 붙었다:\n%s", out)
	}
}

func TestRenderMenuMarksTheCursor(t *testing.T) {
	out := RenderMenu(packs(2), Status{}, 100, Cursor{Index: 1})
	if !strings.Contains(out, "> 2. 이어하기") {
		t.Errorf("가리키는 줄에 표시가 없다:\n%s", out)
	}
	// 표시는 한 줄에만 붙는다. 줄 맨 앞만 센다 — 「선택 >>」에도 같은
	// 두 글자가 들어 있다.
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "> ") || strings.Contains(l, "  > ") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("표시가 %d개다. 한 줄만 가리켜야 한다:\n%s", n, out)
	}
}

func TestRenderMenuKeepsWidthWithCursor(t *testing.T) {
	// 표시를 달아 폭이 변하면 오른쪽 칸이 흔들린다.
	plain := RenderMenu(packs(2), Status{}, 128, Cursor{Index: -1})
	marked := RenderMenu(packs(2), Status{}, 128, Cursor{Index: 1})
	for i, a := range strings.Split(plain, "\n") {
		b := strings.Split(marked, "\n")[i]
		if Width(a) != Width(b) {
			t.Errorf("%d번째 줄 폭이 %d → %d로 바뀌었다", i, Width(a), Width(b))
		}
	}
}

func TestActionBarBracketsTheChoice(t *testing.T) {
	bar := ActionBar(ResultActions, 2)
	if !strings.Contains(bar, "[다시(R)]") {
		t.Errorf("고른 것에 표시가 없다: %s", bar)
	}
	// 글자 키도 함께 적는다 — 입력기가 켜져 있으면 화살표가, 아니면 글자가 쓰인다.
	for _, want := range []string{"(F)", "(R)", "(B)", "(M)", "(X)"} {
		if !strings.Contains(bar, want) {
			t.Errorf("%s가 없다: %s", want, bar)
		}
	}
}

func TestMoveActionStopsAtEnds(t *testing.T) {
	n := len(ResultActions)
	if got := MoveAction(0, KeyLeft, n); got != 0 {
		t.Errorf("맨 앞에서 왼쪽 = %d", got)
	}
	sel := 0
	for i := 0; i < n+5; i++ {
		sel = MoveAction(sel, KeyRight, n)
	}
	if sel != n-1 {
		t.Errorf("맨 뒤 = %d, 기대 %d", sel, n-1)
	}
	if ActionAt(ResultActions, sel) != CmdQuit {
		t.Error("맨 뒤는 종료여야 한다")
	}
}
