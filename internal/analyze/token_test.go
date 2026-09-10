package analyze

import (
	"testing"

	"github.com/noel88/tsuzuri/internal/pack"
)

func jaTok(t *testing.T) Tokenizer {
	t.Helper()
	tk, err := NewTokenizer(pack.KoToJa)
	if err != nil {
		t.Fatalf("NewTokenizer(ko2ja): %v", err)
	}
	return tk
}

func koTok(t *testing.T) Tokenizer {
	t.Helper()
	tk, err := NewTokenizer(pack.JaToKo)
	if err != nil {
		t.Fatalf("NewTokenizer(ja2ko): %v", err)
	}
	return tk
}

func TestJapaneseBaseForm(t *testing.T) {
	tokens := jaTok(t).Tokenize("長く座っていた思わなかった")
	for _, want := range []string{"長い", "座る", "いる", "思う"} {
		if !hasBase(tokens, want) {
			t.Errorf("기본형 %q를 찾지 못했다: %v", want, bases(tokens))
		}
	}
}

func TestJapanesePOSDetail(t *testing.T) {
	// 「静か」는 名詞/形容動詞語幹이다. な형용사 오류 탐지에 이 세분류가 필요하다.
	tokens := jaTok(t).Tokenize("静かで")
	var found bool
	for _, tk := range tokens {
		if tk.Surface == "静か" {
			found = true
			if tk.POS1 != "形容動詞語幹" {
				t.Errorf("POS1 = %q, 기대 \"形容動詞語幹\"", tk.POS1)
			}
		}
	}
	if !found {
		t.Errorf("「静か」 토큰이 없다: %v", surfaces(tokens))
	}
}

func TestJapaneseSymbolsNotUnknown(t *testing.T) {
	tokens := jaTok(t).Tokenize("カフェは静かだ。")
	for _, tk := range tokens {
		if tk.POS == "記号" && !tk.IsSymbol() {
			t.Errorf("기호 %q가 IsSymbol이 아니다", tk.Surface)
		}
	}
}

func TestKoreanStemFromExpression(t *testing.T) {
	// 「해서」는 VV+EC, Expression이 "하/VV/*+아서/EC/*" 이므로 어간은 "하"
	tokens := koTok(t).Tokenize("조용해서")
	if !hasBase(tokens, "하") {
		t.Errorf("「해서」에서 어간 「하」를 뽑지 못했다: %v", bases(tokens))
	}
}

func TestKoreanSpacesAreNotUnknown(t *testing.T) {
	// ko-dic은 띄어쓰기마다 SP 토큰을 UNKNOWN으로 낸다.
	// 오타가 아니므로 미지어로 세면 안 된다.
	tokens := koTok(t).Tokenize("어제 처음 간 카페에 갔다")
	for _, tk := range tokens {
		if tk.Unknown && tk.IsSymbol() {
			t.Errorf("기호/공백 토큰 %q가 미지어로 남아 있다 (POS=%s)", tk.Surface, tk.POS)
		}
	}
	if len(UnknownTokensOf(tokens)) != 0 {
		t.Errorf("정상 문장에 미지어가 없어야 한다: %q", UnknownTokensOf(tokens))
	}
}

func TestKoreanIsContentExcludesParticlesAndSymbols(t *testing.T) {
	tokens := koTok(t).Tokenize("카페에 갔다.")
	for _, tk := range tokens {
		switch tk.POS {
		case "JKB", "JKS", "SF", "SP":
			if tk.IsContent() {
				t.Errorf("%q(%s)가 내용어로 분류됐다", tk.Surface, tk.POS)
			}
		}
	}
}

func TestKoreanIsContentIncludesXR(t *testing.T) {
	// 「조용해서」의 「조용」은 XR(어근)이다. 내용어로 세야 한다.
	tokens := koTok(t).Tokenize("조용해서")
	var sawXR bool
	for _, tk := range tokens {
		if tk.POS == "XR" {
			sawXR = true
			if !tk.IsContent() {
				t.Errorf("XR 「%s」이 내용어가 아니다", tk.Surface)
			}
		}
	}
	if !sawXR {
		t.Skip("이 문장에 XR이 없다 — 사전 버전 차이")
	}
}

// --- 헬퍼 ---

func UnknownTokensOf(ts []Token) []string {
	out := []string{}
	for _, t := range ts {
		if t.Unknown {
			out = append(out, t.Surface)
		}
	}
	return out
}

func hasBase(ts []Token, base string) bool {
	for _, t := range ts {
		if t.Base == base {
			return true
		}
	}
	return false
}

func bases(ts []Token) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Base)
	}
	return out
}

func surfaces(ts []Token) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Surface)
	}
	return out
}
