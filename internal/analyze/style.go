package analyze

import (
	"strings"

	"github.com/noel88/tsuzuri/internal/pack"
)

// DetectStyle은 답안의 문체를 추정한다.
// 판단 근거가 없으면 빈 문자열을 돌려준다 —
// 모른다고 말하는 편이 틀리게 단정하는 것보다 낫다.
func DetectStyle(tokens []Token) pack.Style {
	// 일본어: 정중체·보통체 표지를 문장 어디서든 찾는다.
	for _, t := range tokens {
		switch t.Base {
		case "です", "ます":
			return pack.StylePolite
		case "だ", "である":
			return pack.StylePlain
		}
	}

	// 한국어: 마지막 어미로 판단한다.
	//
	// EF(종결어미)만 보면 안 된다. ko-dic은 「있었다.」의 「다」를
	// EC(연결어미)로 태깅한다 — 문장 끝인데도 그렇다.
	// 그래서 기호를 제외한 마지막 어미 토큰을 본다.
	if last, ok := lastEnding(tokens); ok {
		if isKoreanPolite(last.Surface) {
			return pack.StylePolite
		}
		return pack.StylePlain
	}

	// 일본어 보통체는 표지 없이 「〜た」「〜る」로 끝나는 일이 많다.
	// 정중체 표지가 하나도 없고 용언으로 끝나면 보통체로 본다.
	if last, ok := lastMeaningful(tokens); ok {
		switch last.POS {
		case "助動詞", "動詞", "形容詞":
			return pack.StylePlain
		}
	}
	return ""
}

// lastEnding은 기호를 제외한 마지막 한국어 어미(E로 시작하는 태그) 토큰이다.
func lastEnding(tokens []Token) (Token, bool) {
	for i := len(tokens) - 1; i >= 0; i-- {
		t := tokens[i]
		if t.IsSymbol() {
			continue
		}
		if strings.HasPrefix(t.POS, "E") {
			return t, true
		}
		// 어미가 아닌 실질 형태소를 만나면 더 볼 필요가 없다.
		return Token{}, false
	}
	return Token{}, false
}

// lastMeaningful은 기호를 제외한 마지막 토큰이다.
func lastMeaningful(tokens []Token) (Token, bool) {
	for i := len(tokens) - 1; i >= 0; i-- {
		if !tokens[i].IsSymbol() {
			return tokens[i], true
		}
	}
	return Token{}, false
}

func isKoreanPolite(surface string) bool {
	for _, suf := range []string{"요", "습니다", "ㅂ니다", "십시오", "세요", "ᄇ니다"} {
		if strings.HasSuffix(surface, suf) {
			return true
		}
	}
	return false
}

// StyleMismatch는 답안의 문체가 문제가 요구한 문체와 어긋나는지 본다.
// 판단할 수 없으면 false다 — 확실할 때만 짚는다.
func StyleMismatch(tokens []Token, want pack.Style) bool {
	got := DetectStyle(tokens)
	if got == "" || want == "" {
		return false
	}
	return got != want
}
