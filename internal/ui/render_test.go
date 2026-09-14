package ui

import (
	"strings"
	"testing"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/pack"
)

func sampleProblem() pack.Problem {
	return pack.Problem{
		ID:        "p047",
		Dir:       pack.KoToJa,
		Level:     "N3",
		Topic:     "일상",
		Prompt:    "어제 처음 간 카페가 생각보다 조용해서 오래 앉아 있었다.",
		Reference: "昨日初めて行ったカフェが思ったより静かで、長く座っていた。",
		KeyPoints: []string{"初めて", "思ったより", "〜ていた"},
		Style:     pack.StylePlain,
	}
}

func sampleStatus() Status {
	return Status{Index: 12, Total: 47, QueueLen: 12, Online: false}
}

func noisyAnalysis() analyze.Analysis {
	return analyze.Analysis{
		Covered:       []string{"初めて", "〜ていた"},
		Missing:       []string{"思ったより"},
		Flags:         []analyze.Flag{{Text: "静かく", Why: "な형용사는 「静かで」로 활용한다"}},
		StyleMismatch: true,
		DetectedStyle: pack.StylePolite,
		RefOnly:       []string{"思う", "長い"},
		AnsOnly:       []string{"ずっと"},
		LenRatio:      0.82,
	}
}

func TestRenderProblemHidesReference(t *testing.T) {
	out := RenderProblem(sampleProblem(), sampleStatus(), 92)

	if !strings.Contains(out, "어제 처음 간 카페") {
		t.Error("제시문이 보여야 한다")
	}
	if strings.Contains(out, "昨日初めて行った") {
		t.Fatal("참조 번역이 답안 입력 전에 노출되면 안 된다 — 드릴이 성립하지 않는다")
	}
	if !strings.Contains(out, "p047") || !strings.Contains(out, "N3") {
		t.Errorf("머리말에 ID와 레벨이 있어야 한다:\n%s", out)
	}
	if !strings.Contains(out, "12/47") {
		t.Errorf("진행 상황이 보여야 한다:\n%s", out)
	}
}

func TestRenderResultShowsAnswerAndReference(t *testing.T) {
	out := RenderResult(sampleProblem(), "昨日初めて行ったカフェは静かくて、ずっと座ってました。",
		noisyAnalysis(), sampleStatus(), 92)

	for _, want := range []string{
		"静かくて", "思ったより", "初めて", "な형용사", "ずっと", "12건", "오프라인", "정중체",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("출력에 %q가 없다:\n%s", want, out)
		}
	}
}

func TestRenderResultShowsReferenceAfterPrompt(t *testing.T) {
	out := RenderResult(sampleProblem(), "답안", noisyAnalysis(), sampleStatus(), 92)
	pi := strings.Index(out, "어제 처음 간")
	ri := strings.Index(out, "참조:")
	if pi < 0 || ri < 0 {
		t.Fatalf("제시문과 참조가 모두 있어야 한다:\n%s", out)
	}
	if ri < pi {
		t.Error("참조는 제시문 뒤에 와야 한다")
	}
}

func TestRenderResultHasNoVerdictWords(t *testing.T) {
	out := RenderResult(sampleProblem(), "답안", noisyAnalysis(), sampleStatus(), 92)
	for _, banned := range []string{"점수", "정답", "오답", "score", "%"} {
		if strings.Contains(out, banned) {
			t.Errorf("판정 표현 %q가 화면에 나오면 안 된다 (스펙 §5.4):\n%s", banned, out)
		}
	}
}

func TestRenderResultQuietWhenNothingToFlag(t *testing.T) {
	quiet := analyze.Analysis{Covered: []string{"初めて"}, LenRatio: 1.0}
	out := RenderResult(sampleProblem(), "답안", quiet, sampleStatus(), 92)

	if strings.Contains(out, "⚠") {
		t.Errorf("짚을 게 없으면 ⚠ 줄이 없어야 한다:\n%s", out)
	}
	if strings.Contains(out, "문체") {
		t.Errorf("문체가 맞으면 그 줄이 없어야 한다:\n%s", out)
	}
	if !strings.Contains(out, "✓ 「初めて」") {
		t.Errorf("커버된 표현은 보여야 한다:\n%s", out)
	}
}

func TestRenderResultCompletelyQuiet(t *testing.T) {
	// 커버된 것도 없고 짚을 것도 없으면 지적 블록 자체가 없다.
	out := RenderResult(sampleProblem(), "답안", analyze.Analysis{}, sampleStatus(), 92)
	if strings.Contains(out, "✓") || strings.Contains(out, "✗") || strings.Contains(out, "⚠") {
		t.Errorf("지적 블록이 통째로 없어야 한다:\n%s", out)
	}
}

func TestRenderKeepsLineWidthWithinTerminal(t *testing.T) {
	for _, termW := range []int{60, 76, 92, 120} {
		out := RenderResult(sampleProblem(), "昨日初めて行ったカフェは静かくて、ずっと座ってました。",
			noisyAnalysis(), sampleStatus(), termW)
		for _, line := range strings.Split(out, "\n") {
			if Width(line) > termW {
				t.Errorf("폭 %d에서 줄이 넘친다 (%d칸): %q", termW, Width(line), line)
			}
		}
	}
}

func TestLabeledAlignsContinuationLines(t *testing.T) {
	// 「나」와 「참조」의 본문 시작 위치가 같아야 대조가 된다.
	long := strings.Repeat("あ", 60)
	me := labeled("나  : ", long, 40)
	ref := labeled("참조: ", long, 40)

	if len(me) < 2 || len(ref) < 2 {
		t.Fatalf("여러 줄로 접혀야 한다: %d / %d", len(me), len(ref))
	}
	mi := Width(me[0]) - Width(strings.TrimLeft(me[0], " "))
	ri := Width(ref[0]) - Width(strings.TrimLeft(ref[0], " "))
	if mi != ri {
		t.Errorf("라벨 폭이 달라 시작 위치가 어긋난다: %d vs %d", mi, ri)
	}
	// 이어지는 줄도 본문 시작 위치가 유지되어야 한다.
	if Width(me[1])-Width(strings.TrimLeft(me[1], " ")) != Width("  나  : ") {
		t.Errorf("이어지는 줄 들여쓰기가 어긋난다: %q", me[1])
	}
}

func TestWrapRespectsDisplayWidth(t *testing.T) {
	got := wrap(strings.Repeat("あ", 10), 10)
	if len(got) != 2 {
		t.Fatalf("10칸에 전각 5자씩 두 줄이어야 한다: %d줄 %v", len(got), got)
	}
	for _, l := range got {
		if Width(l) > 10 {
			t.Errorf("줄 폭 %d가 10을 넘는다: %q", Width(l), l)
		}
	}
}

func TestWrapSplitsOnNewlines(t *testing.T) {
	// :e 에디터로 쓴 여러 줄 답안이 그대로 들어온다.
	got := wrap("첫 줄\n둘째 줄", 40)
	if len(got) != 2 {
		t.Fatalf("두 줄이어야 한다: %d줄 %q", len(got), got)
	}
	for _, l := range got {
		if strings.Contains(l, "\n") {
			t.Errorf("결과에 개행이 남으면 안 된다: %q", l)
		}
	}
}

func TestLabeledIndentsEveryLineOfMultilineAnswer(t *testing.T) {
	// 개행을 폭 1로 취급하면 둘째 줄이 들여쓰기를 잃어 나/참조 대조가 깨진다.
	got := labeled("나  : ", "내일은 비가 올 거예요.\n그리고 바람도 불 거예요.", 60)
	if len(got) != 2 {
		t.Fatalf("두 줄이어야 한다: %d줄 %q", len(got), got)
	}
	indent := func(s string) int { return Width(s) - Width(strings.TrimLeft(s, " ")) }
	if indent(got[1]) != Width("  나  : ") {
		t.Errorf("이어지는 줄이 라벨 폭만큼 들여써져야 한다: %q (들여쓰기 %d)",
			got[1], indent(got[1]))
	}
}

func TestRenderResultFitsTerminalWithMultilineAnswer(t *testing.T) {
	answer := "내일은 비가 올 거예요.\n그리고 바람도 불 거예요.\n우산을 챙기세요."
	for _, termW := range []int{60, 76, 92} {
		out := RenderResult(sampleProblem(), answer, noisyAnalysis(), sampleStatus(), termW)
		for _, line := range strings.Split(out, "\n") {
			if Width(line) > termW {
				t.Errorf("폭 %d에서 넘친다 (%d칸): %q", termW, Width(line), line)
			}
			if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "┌") &&
				!strings.HasPrefix(line, "│") && !strings.HasPrefix(line, "└") &&
				!strings.HasPrefix(line, "─") {
				t.Errorf("가운데 여백을 잃은 줄이 있다: %q", line)
			}
		}
	}
}
