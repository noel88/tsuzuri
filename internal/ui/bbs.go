package ui

import "strings"

// PC통신(하이텔) 스타일 화면 요소들.
//
// 이 스타일을 고른 이유는 취향만이 아니다. 하이텔은 애초에 번호를 쳐서
// 이동하는 방식인데, 우리도 mozc IME 때문에 raw mode를 쓸 수 없어
// 조작이 "타이핑 + Enter"다. 같은 제약에서 나온 같은 답이라 잘 맞는다.
//
// 모든 함수는 표시폭(Width) 기준으로 계산한다. rune 개수로 세면
// CJK가 섞이는 순간 테두리가 어긋난다.

// Banner는 이중선 배너다. 초기화면 상단에 쓴다.
//
//	╔══════════════════════════╗
//	║ 綴 / TSUZURI    부제목    ║
//	╚══════════════════════════╝
func Banner(title, subtitle string, w int) []string {
	return framed("╔", "═", "╗", "║", "╚", "╝", title, subtitle, w)
}

// Frame은 단일선 상자다. 드릴 화면 머리말에 쓴다.
//
//	┌──────────────────────────┐
//	│ p047  ko2ja  N3     12/47│
//	└──────────────────────────┘
func Frame(left, right string, w int) []string {
	return framed("┌", "─", "┐", "│", "└", "┘", left, right, w)
}

func framed(tl, h, tr, v, bl, br, left, right string, w int) []string {
	inner := w - 2
	if inner < 1 {
		inner = 1
	}
	body := " " + left
	tail := right + " "
	gap := inner - Width(body) - Width(tail)
	if gap < 1 {
		// 좁으면 오른쪽을 버린다. 왼쪽(식별자)이 더 중요하다.
		body = Truncate(body, inner-1)
		tail = ""
		gap = inner - Width(body)
	}
	return []string{
		tl + strings.Repeat(h, inner) + tr,
		v + body + strings.Repeat(" ", gap) + tail + v,
		bl + strings.Repeat(h, inner) + br,
	}
}

// Box는 짧은 제목을 감싼 작은 상자다. 하이텔의 「자료실」 같은 구획 제목.
//
//	┌──────────┐
//	│ 자 료 실 │
//	└──────────┘
func Box(label string) []string {
	inner := Width(label) + 2
	return []string{
		"┌" + strings.Repeat("─", inner) + "┐",
		"│ " + label + " │",
		"└" + strings.Repeat("─", inner) + "┘",
	}
}

// MenuItem은 번호 항목과 그 아래 밑줄이다. 하이텔 메뉴의 특징적 형태.
//
//  1. 드릴 시작
//     ──────────────────────
func MenuItem(no, label string, ruleW int) []string {
	return []string{
		"  " + no + ". " + label,
		"  " + strings.Repeat("─", ruleW),
	}
}

// Entry는 우측 목록의 한 줄이다. 번호 · 이름 · 오른쪽 끝 수치.
//
//  41. ko2ja  N3 일상            47
func Entry(no, label, value string, w int) string {
	head := "  " + no + ". " + label
	if value == "" {
		return head
	}
	gap := w - Width(head) - Width(value)
	if gap < 1 {
		gap = 1
	}
	return head + strings.Repeat(" ", gap) + value
}

// TwoCol은 두 칼럼을 나란히 놓는다. 짧은 쪽은 빈 줄로 채운다.
func TwoCol(left, right []string, leftW, gap int) []string {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		fill := leftW - Width(l)
		if fill < 0 {
			fill = 0
		}
		out[i] = strings.TrimRight(l+strings.Repeat(" ", fill+gap)+r, " ")
	}
	return out
}

// Divider는 가로 구분선이다.
func Divider(w int) string {
	return strings.Repeat("─", maxInt(w, 0))
}

// CommandBar는 하단 명령어 안내와 입력 프롬프트다.
//
//	──────────────────────────────────────
//	주요명령(다음 ⏎, 첨삭 F)  메뉴(M)  종료(X)
//	선택 >>
func CommandBar(commands, prompt string, w int) []string {
	return []string{
		" " + Divider(w-2),
		" " + Truncate(commands, w-2),
		" " + prompt + " ",
	}
}

// CommandBarWith는 구분선에 상태를 얹은 명령줄이다.
//
//	───────────── 큐 12건 · 오프라인 ─────────────
//	주요명령(다음 ⏎, 첨삭 F, 다시 R)  메뉴(M)  종료(X)
//	선택 >>
//
// 상태를 명령 문자열에 이어붙이면 좁은 화면에서 명령이 잘려나간다.
// 구분선은 어차피 비어 있으므로 그쪽에 둔다.
func CommandBarWith(status, commands, prompt string, w int) []string {
	// 구분선은 위쪽 상자와 오른쪽 끝을 맞춘다. 앞에 공백을 두면 1칸 어긋난다.
	return []string{
		Rule(" "+status+" ", w),
		" " + Truncate(commands, w-2),
		" " + prompt + " ",
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
