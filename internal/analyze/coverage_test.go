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

// 한국어 보조용언은 붙여 써도 맞다(한글 맞춤법 제47항). 띄어쓰기까지
// 따지면 맞는 답에 "빠짐"이 뜬다.
func TestCoverageIgnoresSpacingVariants(t *testing.T) {
	tk := koTok(t)
	tokens := tk.Tokenize("오래 앉아있었다.")
	covered, missing := Coverage(tokens, []string{"앉아 있"})
	if len(covered) != 1 || len(missing) != 0 {
		t.Errorf("붙여 쓴 보조용언도 맞아야 한다: covered=%v missing=%v", covered, missing)
	}
}

// 생성된 핵심 표현은 사전형(「가져오다」)으로 적히는데, 형태소 분석은
// 어간(「가져오」)만 돌려준다.
func TestCoverageMatchesDictionaryFormKeyPoints(t *testing.T) {
	tk := koTok(t)
	tokens := tk.Tokenize("우산을 가져왔습니다.")
	covered, missing := Coverage(tokens, []string{"가져오다"})
	if len(covered) != 1 || len(missing) != 0 {
		t.Errorf("사전형 핵심 표현도 맞아야 한다: covered=%v missing=%v (기본형 %v)",
			covered, missing, bases(tokens))
	}
}

func TestCoverageStillReportsGenuinelyMissing(t *testing.T) {
	tk := koTok(t)
	tokens := tk.Tokenize("어제 카페에 갔다.")
	_, missing := Coverage(tokens, []string{"생각보다"})
	if len(missing) != 1 {
		t.Errorf("정말 없는 표현은 빠짐으로 나와야 한다: %v", missing)
	}
}
