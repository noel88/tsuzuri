package ui

import (
	"fmt"
	"strings"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/pack"
)

// Status는 화면 하단·상단에 표시할 현재 상태다.
type Status struct {
	Index    int  // 지금 몇 번째 문제인지 (1부터)
	Total    int  // 전체 문제 수
	QueueLen int  // 첨삭 대기 건수
	Feedback int  // 받은 첨삭 건수
	Online   bool // 네트워크 연결 여부
}

// RenderProblem은 답안 입력 전 화면이다.
//
// 참조 번역을 노출하지 않는다. 답을 미리 보여주면 드릴이 성립하지 않는다.
func RenderProblem(p pack.Problem, st Status, termW int) string {
	w := BoxWidth(termW)

	var lines []string
	lines = append(lines, Frame(problemTitle(p), progress(st), w)...)
	lines = append(lines, "")
	lines = append(lines, wrapIndent(p.Prompt, w-4, "  ")...)
	lines = append(lines, "")
	lines = append(lines, CommandBarWith(
		fmt.Sprintf("큐 %d건 · %s", st.QueueLen, connLabel(st.Online)),
		"주요명령(긴 답 :e)  메뉴(M)  종료(X)",
		"답 >>", w)...)

	return Join(Center(lines, termW, w))
}

// RenderResult는 제출 후 화면이다.
//
// 점수도 정오 판정도 없다 (스펙 §5.4).
// ✓ ✗ ⚠ 는 오답 표시가 아니라 확인 요청이다.
// 짚을 것이 없는 줄은 아예 그리지 않는다.
func RenderResult(p pack.Problem, answer string, a analyze.Analysis, st Status, termW int) string {
	w := BoxWidth(termW)
	inner := w - 4

	var lines []string
	lines = append(lines, Frame(problemTitle(p), progress(st), w)...)
	lines = append(lines, "")
	lines = append(lines, wrapIndent(p.Prompt, inner, "  ")...)
	lines = append(lines, "")
	lines = append(lines, labeled("나  : ", answer, inner)...)
	lines = append(lines, labeled("참조: ", p.Reference, inner)...)

	if notes := resultNotes(p, a, inner); len(notes) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+Divider(inner))
		lines = append(lines, notes...)
		lines = append(lines, "  "+Divider(inner))
	}

	lines = append(lines, "")
	lines = append(lines, CommandBarWith(
		fmt.Sprintf("큐 %d건 · %s", st.QueueLen, connLabel(st.Online)),
		"주요명령(다음 ⏎, 첨삭 F, 다시 R)  메뉴(M)  종료(X)",
		"선택 >>", w)...)

	return Join(Center(lines, termW, w))
}

// resultNotes는 지적 줄들을 만든다. 짚을 것이 없으면 빈 슬라이스다.
func resultNotes(p pack.Problem, a analyze.Analysis, inner int) []string {
	var out []string

	// 핵심 표현 — 있는 것과 빠진 것을 한 줄에 모은다.
	var marks []string
	for _, c := range a.Covered {
		marks = append(marks, "✓ 「"+c+"」")
	}
	for _, m := range a.Missing {
		marks = append(marks, "✗ 「"+m+"」 빠짐")
	}
	if len(marks) > 0 {
		out = append(out, wrapIndent(strings.Join(marks, "   "), inner, "  ")...)
	}

	// 미지어·의심 형태
	for _, f := range a.Flags {
		out = append(out, "  ⚠ 「"+f.Text+"」   "+f.Why)
	}

	// 문체 — 어긋날 때만
	if a.StyleMismatch {
		out = append(out, fmt.Sprintf("  ⚠ 문체        답안 %s / 문제 %s",
			styleLabel(a.DetectedStyle), styleLabel(p.Style)))
	}

	// 어휘 차이 — 있을 때만
	var diff []string
	if len(a.RefOnly) > 0 {
		diff = append(diff, "참조에만: "+strings.Join(a.RefOnly, ", "))
	}
	if len(a.AnsOnly) > 0 {
		diff = append(diff, "내 답안에만: "+strings.Join(a.AnsOnly, ", "))
	}
	if len(diff) > 0 {
		out = append(out, wrapIndent("· "+strings.Join(diff, "   "), inner, "  ")...)
	}
	return out
}

func problemTitle(p pack.Problem) string {
	parts := []string{p.ID, string(p.Dir)}
	if p.Level != "" {
		parts = append(parts, p.Level)
	}
	if p.Topic != "" {
		parts = append(parts, p.Topic)
	}
	return strings.Join(parts, "   ")
}

func progress(st Status) string {
	if st.Total <= 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", st.Index, st.Total)
}

func styleLabel(s pack.Style) string {
	switch s {
	case pack.StylePolite:
		return "정중체"
	case pack.StylePlain:
		return "보통체"
	default:
		return "판단 불가"
	}
}

func connLabel(online bool) string {
	if online {
		return "온라인"
	}
	return "오프라인"
}

// labeled는 "나  : 본문" 형태로 쓰되, 이어지는 줄은 라벨 폭만큼 들여쓴다.
// 「나」와 「참조」의 본문 시작 위치가 맞아야 대조가 된다.
func labeled(label, text string, inner int) []string {
	indent := strings.Repeat(" ", Width(label))
	wrapped := wrap(text, inner-Width(label))
	out := make([]string, 0, len(wrapped))
	for i, l := range wrapped {
		if i == 0 {
			out = append(out, "  "+label+l)
		} else {
			out = append(out, "  "+indent+l)
		}
	}
	return out
}

func wrapIndent(text string, width int, prefix string) []string {
	wrapped := wrap(text, width)
	out := make([]string, 0, len(wrapped))
	for _, l := range wrapped {
		out = append(out, prefix+l)
	}
	return out
}

// wrap은 표시폭 기준으로 줄을 나눈다.
// CJK는 단어 경계가 없으므로 폭이 차면 그냥 끊는다.
func wrap(text string, width int) []string {
	if width < 4 {
		width = 4
	}
	var out []string
	var cur strings.Builder
	w := 0
	for _, r := range text {
		rw := RuneWidth(r)
		if w+rw > width {
			out = append(out, cur.String())
			cur.Reset()
			w = 0
		}
		cur.WriteRune(r)
		w += rw
	}
	if cur.Len() > 0 || len(out) == 0 {
		out = append(out, cur.String())
	}
	return out
}
