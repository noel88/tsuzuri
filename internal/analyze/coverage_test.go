package analyze

import (
	"reflect"
	"testing"
)

func TestCoverageMatchesByBaseForm(t *testing.T) {
	tokens := []Token{
		{Surface: "思わ", Base: "思う", POS: "動詞"},
		{Surface: "なかっ", Base: "ない", POS: "助動詞"},
		{Surface: "た", Base: "た", POS: "助動詞"},
	}
	covered, missing := Coverage(tokens, []string{"思う", "初めて"})

	if !reflect.DeepEqual(covered, []string{"思う"}) {
		t.Errorf("covered = %v, 기대 [思う]", covered)
	}
	if !reflect.DeepEqual(missing, []string{"初めて"}) {
		t.Errorf("missing = %v, 기대 [初めて]", missing)
	}
}

func TestCoverageMatchesBySurface(t *testing.T) {
	tokens := []Token{{Surface: "初めて", Base: "初めて", POS: "副詞"}}
	covered, missing := Coverage(tokens, []string{"初めて"})
	if len(covered) != 1 || len(missing) != 0 {
		t.Errorf("covered=%v missing=%v", covered, missing)
	}
}

func TestCoverageStripsTilde(t *testing.T) {
	tokens := []Token{
		{Surface: "て", Base: "て"},
		{Surface: "い", Base: "いる"},
		{Surface: "た", Base: "た"},
	}
	covered, _ := Coverage(tokens, []string{"〜ていた"})
	if len(covered) != 1 {
		t.Errorf("물결표를 뗀 「ていた」가 연결된 표층에서 매칭되어야 한다: %v", covered)
	}
}

func TestCoverageEmptySlicesNotNil(t *testing.T) {
	// JSON으로 저장되므로 nil이 아니라 []여야 한다.
	covered, missing := Coverage(nil, nil)
	if covered == nil || missing == nil {
		t.Errorf("빈 슬라이스여야 한다: covered=%v missing=%v", covered, missing)
	}
}

func TestUnknownTokensSkipsSymbols(t *testing.T) {
	tokens := []Token{
		{Surface: "カフェ", POS: "名詞"},
		{Surface: " ", POS: "SP", Unknown: true},
		{Surface: "오타아", POS: "NNG", Unknown: true},
	}
	got := UnknownTokens(tokens)
	if len(got) != 1 || got[0].Text != "오타아" {
		t.Errorf("기호를 뺀 미지어만 나와야 한다: %+v", got)
	}
	if got[0].Why == "" {
		t.Error("사유가 있어야 한다")
	}
}

func TestUnknownTokensEmptyWhenAllKnown(t *testing.T) {
	got := UnknownTokens([]Token{{Surface: "カフェ"}, {Surface: "は"}})
	if len(got) != 0 {
		t.Errorf("빈 슬라이스여야 한다: %+v", got)
	}
}

// 실제 토크나이저를 통과시키는 검증.
// 「静かくて」가 미지어로는 안 잡히고 패턴으로만 잡힌다는 것을 고정한다.
func TestSuspectFormsCatchesNaAdjectiveMisconjugation(t *testing.T) {
	tokens := jaTok(t).Tokenize("カフェは静かくて、静かでした。")

	if len(UnknownTokens(tokens)) != 0 {
		t.Errorf("IPADIC은 「静かく」를 정상 분해한다 — 미지어가 없어야 한다: %+v",
			UnknownTokens(tokens))
	}

	got := SuspectForms(tokens)
	if len(got) != 1 {
		t.Fatalf("의심 형태 1건이어야 한다: %+v (토큰 %v)", got, surfaces(tokens))
	}
	if got[0].Text != "静かく" {
		t.Errorf("Text = %q, 기대 \"静かく\"", got[0].Text)
	}
	if got[0].Why == "" {
		t.Error("사유가 있어야 한다")
	}
}

func TestSuspectFormsQuietOnCorrectSentence(t *testing.T) {
	tokens := jaTok(t).Tokenize("昨日初めて行ったカフェが思ったより静かで、長く座っていた。")
	if got := SuspectForms(tokens); len(got) != 0 {
		t.Errorf("올바른 문장에서 의심 형태가 나오면 안 된다: %+v", got)
	}
}

func TestSuspectFormsDoesNotFlagNormalKuAdverb(t *testing.T) {
	// 「長く」는 い형용사의 정상 활용이다. 앞이 形容動詞語幹이 아니므로 안 걸린다.
	tokens := jaTok(t).Tokenize("長く座っていた")
	if got := SuspectForms(tokens); len(got) != 0 {
		t.Errorf("정상 활용을 짚으면 안 된다: %+v", got)
	}
}
