package analyze

import (
	"reflect"
	"testing"
)

func TestVocabDiffComparesContentWordsOnly(t *testing.T) {
	answer := []Token{
		{Surface: "カフェ", Base: "カフェ", POS: "名詞"},
		{Surface: "は", Base: "は", POS: "助詞"},
		{Surface: "ずっと", Base: "ずっと", POS: "副詞"},
	}
	reference := []Token{
		{Surface: "カフェ", Base: "カフェ", POS: "名詞"},
		{Surface: "が", Base: "が", POS: "助詞"},
		{Surface: "長く", Base: "長い", POS: "形容詞"},
	}
	refOnly, ansOnly := VocabDiff(answer, reference)

	if !reflect.DeepEqual(refOnly, []string{"長い"}) {
		t.Errorf("refOnly = %v, 기대 [長い]", refOnly)
	}
	if !reflect.DeepEqual(ansOnly, []string{"ずっと"}) {
		t.Errorf("ansOnly = %v, 기대 [ずっと]", ansOnly)
	}
}

func TestVocabDiffEmptyWhenIdentical(t *testing.T) {
	toks := []Token{{Surface: "カフェ", Base: "カフェ", POS: "名詞"}}
	refOnly, ansOnly := VocabDiff(toks, toks)
	if len(refOnly) != 0 || len(ansOnly) != 0 {
		t.Errorf("동일하면 차이가 없어야 한다: %v / %v", refOnly, ansOnly)
	}
}

func TestVocabDiffDeduplicates(t *testing.T) {
	answer := []Token{
		{Surface: "ずっと", Base: "ずっと", POS: "副詞"},
		{Surface: "ずっと", Base: "ずっと", POS: "副詞"},
	}
	_, ansOnly := VocabDiff(answer, nil)
	if len(ansOnly) != 1 {
		t.Errorf("중복은 한 번만: %v", ansOnly)
	}
}

func TestVocabDiffViaTokenizer(t *testing.T) {
	tk := jaTok(t)
	ans := tk.Tokenize("昨日初めて行ったカフェは静かでした。")
	ref := tk.Tokenize("昨日初めて行ったカフェが思ったより静かで、長く座っていた。")

	refOnly, _ := VocabDiff(ans, ref)
	if !containsStr(refOnly, "思う") {
		t.Errorf("참조에만 있는 「思う」가 나와야 한다: %v", refOnly)
	}
	if !containsStr(refOnly, "長い") {
		t.Errorf("참조에만 있는 「長い」가 나와야 한다: %v", refOnly)
	}
	if containsStr(refOnly, "カフェ") {
		t.Errorf("양쪽에 있는 「カフェ」가 차이로 잡히면 안 된다: %v", refOnly)
	}
}

func TestLenRatio(t *testing.T) {
	// 표층이 빈 토큰은 기호로 취급되어 세지 않는다. 실제 토큰처럼 채운다.
	mk := func(n int) []Token {
		ts := make([]Token, n)
		for i := range ts {
			ts[i] = Token{Surface: "あ", Base: "あ", POS: "名詞"}
		}
		return ts
	}
	if got := LenRatio(mk(8), mk(10)); got != 0.8 {
		t.Errorf("LenRatio = %v, 기대 0.8", got)
	}
}

func TestLenRatioIgnoresSymbols(t *testing.T) {
	answer := []Token{{Surface: "A", POS: "名詞"}, {Surface: "。", POS: "記号"}}
	reference := []Token{{Surface: "A", POS: "名詞"}}
	if got := LenRatio(answer, reference); got != 1.0 {
		t.Errorf("기호를 세면 안 된다: %v", got)
	}
}

func TestLenRatioZeroReference(t *testing.T) {
	if got := LenRatio([]Token{{Surface: "A", POS: "名詞"}}, nil); got != 0 {
		t.Errorf("참조가 비면 0: %v", got)
	}
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
