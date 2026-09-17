package ui

import (
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
	tsync "github.com/noel88/tsuzuri/internal/sync"
)

// RenderFeedback은 수령한 첨삭 하나를 보여준다.
//
// error와 nuance를 다른 표식으로 구분한다. 뉘앙스 제안을 오류처럼
// 보여주면 학습자가 맞은 답을 틀린 것으로 받아들인다 (스펙 §5.4).
func RenderFeedback(p pack.Problem, a store.Attempt, f tsync.Feedback, st Status, termW int) string {
	w := BoxWidth(termW)
	inner := w - 4

	var lines []string
	lines = append(lines, Frame(problemTitle(p), indexLabel(st), w)...)
	lines = append(lines, "")
	lines = append(lines, wrapIndent(p.Prompt, inner, "  ")...)
	lines = append(lines, "")
	lines = append(lines, labeled("나  : ", a.Answer, inner)...)
	if f.Corrected != "" {
		lines = append(lines, labeled("첨삭: ", f.Corrected, inner)...)
	}
	lines = append(lines, labeled("참조: ", p.Reference, inner)...)

	if notes := feedbackNotes(f, inner); len(notes) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+Divider(inner))
		lines = append(lines, notes...)
		lines = append(lines, "  "+Divider(inner))
	}
	if f.Overall != "" {
		lines = append(lines, "")
		lines = append(lines, wrapIndent(f.Overall, inner, "  ")...)
	}

	lines = append(lines, "")
	lines = append(lines, CommandBarWith(
		"첨삭",
		"주요명령(다음 ⏎)  메뉴(M)  종료(X)",
		"선택 >>", w)...)

	return Page(lines, termW, w)
}

// feedbackNotes는 error를 먼저, nuance를 나중에 그린다.
// 표식도 다르다 — ✗는 틀린 것, ·는 더 나은 선택의 제안이다.
func feedbackNotes(f tsync.Feedback, inner int) []string {
	var out []string
	for _, lv := range []string{tsync.LevelError, tsync.LevelNuance} {
		for _, n := range f.Notes {
			if n.Level != lv {
				continue
			}
			mark := "✗"
			if lv == tsync.LevelNuance {
				mark = "·"
			}
			out = append(out, wrapIndent(mark+" 「"+n.Span+"」   "+n.Why, inner, "  ")...)
		}
	}
	return out
}
