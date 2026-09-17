package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/pack"
)

// Clear는 화면을 지우고 커서를 맨 위로 옮긴다.
//
// 지우지 않으면 앱을 켜기 전의 셸 출력이 위에 남아 화면이 어수선하다.
// fbterm에서 기본 ANSI(ESC[2J, ESC[H)는 동작한다 — M0에서 글리프와 함께
// 확인했다. 혹시 어긋나는 터미널이 있으면 TSUZURI_NO_CLEAR=1로 끌 수 있다.
func Clear(w io.Writer) {
	if !ClearEnabled() {
		fmt.Fprintln(w)
		return
	}
	fmt.Fprint(w, "\033[2J\033[H")
}

// ClearEnabled는 화면을 지우는지 알려준다.
//
// 지우는 동안에는 방금 띄운 메시지가 다음 화면에 덮여 사라진다. 부르는
// 쪽은 이것을 보고 읽을 틈을 줄지 정한다.
func ClearEnabled() bool {
	return os.Getenv("TSUZURI_NO_CLEAR") == ""
}

// Status는 화면 하단·상단에 표시할 현재 상태다.
type Status struct {
	Index    int // 지금 몇 번째 문제인지 (1부터)
	Total    int // 전체 문제 수
	QueueLen int // 첨삭 대기 건수
	Feedback int // 받은 첨삭 건수
	Due      int // 복습할 문제 수
	Packs    int // 자료실의 팩 수 (접어 둔 것 포함)

	// StartKey는 「드릴 시작」이 열 팩의 번호다.
	StartKey string
	Online   bool // 네트워크 연결 여부

	// Warn은 화면 위에 한 번 띄울 알림이다.
	//
	// 화면 밖에 따로 찍으면 안 된다. 본문이 화면을 꽉 채우는 긴 문항에서는
	// 그 줄이 위로 밀려 올라가 보이지 않는다 — 사용자는 친 답안이 사라진
	// 것만 보고 이유는 못 본다.
	Warn string

	// Now는 「며칠 뒤」를 계산할 기준 시각이다. 비면 time.Now를 쓴다.
	// 테스트가 화면을 고정된 시각으로 그리기 위해 받는다.
	Now time.Time
}

// RenderProblem은 답안 입력 전 화면이다.
//
// 참조 번역을 노출하지 않는다. 답을 미리 보여주면 드릴이 성립하지 않는다.
// sel이 0 이상이면 명령 안내 대신 고르기 줄을 그린다.
func RenderProblem(p pack.Problem, st Status, termW int, sel int) string {
	w := BoxWidth(termW)

	var lines []string
	lines = append(lines, Frame(problemTitle(p), indexLabel(st), w)...)
	if st.Warn != "" {
		lines = append(lines, "")
		lines = append(lines, wrapIndent("! "+st.Warn, w-4, "  ")...)
	}
	lines = append(lines, "")
	lines = append(lines, wrapIndent(p.Prompt, w-4, "  ")...)
	lines = append(lines, "")
	commands := "주요명령(긴 답 :e)  메뉴(M)  종료(X)  — 빈 줄에서 ⏎ 를 누르면 고를 수 있습니다"
	if p.IsLong() {
		commands = "긴 글입니다. 빈 줄에서 ⏎ 를 누르면 편집기가 열립니다.  메뉴(M)  종료(X)"
	}
	prompt := "답 >>"
	if sel >= 0 {
		commands = ActionBar(AnswerActions, sel)
		prompt = "선택 >>"
	}
	lines = append(lines, CommandBarWith(
		fmt.Sprintf("큐 %d건 · %s", st.QueueLen, connLabel(st.Online)),
		commands,
		prompt, w)...)

	return Page(lines, termW, w)
}

// RenderResult는 제출 후 화면이다.
//
// 점수도 정오 판정도 없다 (스펙 §5.4).
// ✓ ✗ ⚠ 는 오답 표시가 아니라 확인 요청이다.
// 짚을 것이 없는 줄은 아예 그리지 않는다.
func RenderResult(p pack.Problem, answer string, a analyze.Analysis, st Status, termW int, sel int) string {
	w := BoxWidth(termW)
	inner := w - 4

	var lines []string
	lines = append(lines, Frame(problemTitle(p), indexLabel(st), w)...)
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
		ActionBar(ResultActions, sel),
		"선택 >>", w)...)

	return Page(lines, termW, w)
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

func indexLabel(st Status) string {
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
//
// 원래 있던 개행은 먼저 나눈다. :e로 연 에디터에서 쓴 여러 줄 답안이나
// LLM이 돌려준 여러 줄 총평이 그대로 들어오는데, 개행을 폭 1짜리 문자로
// 취급하면 그 줄만 들여쓰기와 가운데 여백을 잃어 나/참조 대조가 깨진다.
func wrap(text string, width int) []string {
	if width < 4 {
		width = 4
	}
	var out []string
	for _, seg := range strings.Split(text, "\n") {
		out = append(out, wrapSegment(strings.TrimRight(seg, "\r"), width)...)
	}
	return out
}

func wrapSegment(text string, width int) []string {
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
