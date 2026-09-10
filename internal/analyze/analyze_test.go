package analyze

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noel88/tsuzuri/internal/pack"
)

func sampleProblem() pack.Problem {
	return pack.Problem{
		ID:        "p001",
		Dir:       pack.KoToJa,
		Prompt:    "어제 처음 간 카페가 생각보다 조용해서 오래 앉아 있었다.",
		Reference: "昨日初めて行ったカフェが思ったより静かで、長く座っていた。",
		KeyPoints: []string{"初めて", "思ったより", "〜ていた"},
		Style:     pack.StylePlain,
	}
}

func TestAnalyzeEndToEnd(t *testing.T) {
	a, err := New(pack.KoToJa)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// 스펙 §6.3 화면 예시와 같은 답안이다.
	got := a.Analyze(sampleProblem(), "昨日初めて行ったカフェは静かくて、ずっと座ってました。")

	if !containsStr(got.Covered, "初めて") {
		t.Errorf("「初めて」가 covered에 있어야 한다: %v", got.Covered)
	}
	if !containsStr(got.Missing, "思ったより") {
		t.Errorf("「思ったより」가 missing에 있어야 한다: %v", got.Missing)
	}
	if !hasFlag(got.Flags, "静かく") {
		t.Errorf("「静かく」가 짚여야 한다: %+v", got.Flags)
	}
	if !got.StyleMismatch {
		t.Errorf("「ました」(정중체) vs plain 요구 = 불일치 (detected=%q)", got.DetectedStyle)
	}
	if !containsStr(got.RefOnly, "長い") {
		t.Errorf("참조에만 있는 「長い」: %v", got.RefOnly)
	}
	if got.LenRatio <= 0 {
		t.Errorf("LenRatio가 계산되어야 한다: %v", got.LenRatio)
	}
	if got.Quiet() {
		t.Error("짚을 게 있는데 Quiet이면 안 된다")
	}
}

func TestAnalyzeQuietOnGoodAnswer(t *testing.T) {
	a, err := New(pack.KoToJa)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// 참조와 동일한 답안 — 짚을 것이 없어야 한다.
	got := a.Analyze(sampleProblem(), sampleProblem().Reference)

	if !got.Quiet() {
		t.Errorf("참조와 같은 답안은 조용해야 한다: missing=%v flags=%+v style=%v",
			got.Missing, got.Flags, got.StyleMismatch)
	}
}

func TestAnalyzeKoreanDirection(t *testing.T) {
	a, err := New(pack.JaToKo)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := pack.Problem{
		ID:        "p010",
		Dir:       pack.JaToKo,
		Prompt:    "昨日カフェに行った。",
		Reference: "어제 카페에 갔다.",
		KeyPoints: []string{"카페"},
		Style:     pack.StylePlain,
	}
	got := a.Analyze(p, "어제 카페에 갔습니다.")

	if !containsStr(got.Covered, "카페") {
		t.Errorf("「카페」가 covered에 있어야 한다: %v", got.Covered)
	}
	if !got.StyleMismatch {
		t.Errorf("「갔습니다」(정중체) vs plain 요구 = 불일치 (detected=%q)", got.DetectedStyle)
	}
}

func TestAnalysisSerializesWithoutScore(t *testing.T) {
	// 저장은 JSON이다. 점수 계열 필드가 새어 들어가지 않는지 고정한다.
	b, err := json.Marshal(Analysis{})
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"score", "correct", "passed", "accuracy", "grade"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("판정 필드 %q가 있으면 안 된다 (스펙 §5.4): %s", banned, b)
		}
	}
}

func hasFlag(fs []Flag, text string) bool {
	for _, f := range fs {
		if f.Text == text {
			return true
		}
	}
	return false
}
