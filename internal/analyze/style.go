package analyze

import (
	"strings"

	"github.com/noel88/tsuzuri/internal/pack"
)

// DetectStyle은 답안의 문체를 추정한다.
// 판단 근거가 없으면 빈 문자열을 돌려준다 —
// 모른다고 말하는 편이 틀리게 단정하는 것보다 낫다.
func DetectStyle(tokens []Token) pack.Style {
	// 한국어 어미가 보이면 한국어로 판단한다.
	if last, ok := lastEnding(tokens); ok {
		if isKoreanPolite(last.Surface) {
			return pack.StylePolite
		}
		return pack.StylePlain
	}

	// 일본어: 정중체 표지를 문장 어디서든 먼저 찾는다.
	//
	// 보통체 표지를 같이 훑으면 안 된다. IPADIC은 연체형 「な」와 중지형
	// 「で」에도 기본형 「だ」를 주기 때문에, 「静かなカフェで…읽었습니다」처럼
	// 정중체 문장도 첫 「な」에서 보통체로 단정하게 된다.
	for _, t := range tokens {
		if t.Base == "です" || t.Base == "ます" {
			return pack.StylePolite
		}
	}

	// 보통체 표지는 문장을 끝맺는 자리에서만 인정한다.
	last, ok := lastMeaningful(tokens)
	if !ok {
		return ""
	}
	if last.Base == "だ" || last.Base == "である" {
		return pack.StylePlain
	}
	// 일본어 보통체는 표지 없이 「〜た」「〜る」로 끝나는 일이 많다.
	switch last.POS {
	case "助動詞", "動詞", "形容詞":
		return pack.StylePlain
	}
	return ""
}

// lastEnding은 기호를 제외한 마지막 한국어 어미 토큰이다.
//
// 어미 태그만 보면 안 된다. ko-dic은 「있었다.」의 「다」를 문장 끝인데도
// EC(연결어미)로 태깅하고, 「입니다」「간다」처럼 어간과 어미가 붙은 토큰에는
// VCP+EF, VV+EC 같은 복합 태그를 준다. 복합 태그의 성분을 봐야 한다.
func lastEnding(tokens []Token) (Token, bool) {
	for i := len(tokens) - 1; i >= 0; i-- {
		t := tokens[i]
		if t.IsSymbol() {
			continue
		}
		if hasEndingTag(t.POS) {
			return t, true
		}
		// 어미를 달고 있지 않은 실질 형태소를 만나면 더 볼 필요가 없다.
		return Token{}, false
	}
	return Token{}, false
}

// hasEndingTag는 복합 태그에 어미 성분(E로 시작)이 있는지 본다.
func hasEndingTag(pos string) bool {
	for _, part := range strings.Split(pos, "+") {
		if strings.HasPrefix(part, "E") {
			return true
		}
	}
	return false
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

// koreanPoliteEndings는 정중체로 끝맺는 어미들이다.
//
// 「니다」와 「니까」는 「입니다」「갔습니까」처럼 어간과 붙은 토큰까지
// 잡기 위한 것이다. 「하니까」 같은 연결어미와 겹치지 않도록, 어미를 달고
// 있는 마지막 토큰에만 적용한다.
var koreanPoliteEndings = []string{"요", "십시오", "세요"}

// 「ㅂ니다」「ㅂ시다」꼴을 잡을 때 앞 음절에서 찾는 종성.
const jongseongBieup = 17 // ᆸ

// isKoreanPolite는 어미 표층이 정중체인지 본다.
//
// 「갑시다」의 「ㅂ」은 앞 음절 「갑」에 합쳐져 있어서 글자로는 못 찾는다.
// 한글 음절을 풀어 종성을 직접 본다 — 그러지 않으면 정중체로 제대로 답한
// 학습자가 문체 어긋남 지적을 받는다.
//
// 「니다」「니까」를 그냥 받으면 안 된다. 「그렇다니까」 같은 반말이 정중체로
// 잡힌다. 앞이 「습」이거나 종성이 ㅂ일 때만 정중체다.
func isKoreanPolite(surface string) bool {
	for _, suf := range koreanPoliteEndings {
		if strings.HasSuffix(surface, suf) {
			return true
		}
	}
	for _, suf := range []string{"니다", "니까", "시다"} {
		if !strings.HasSuffix(surface, suf) {
			continue
		}
		head := []rune(strings.TrimSuffix(surface, suf))
		if len(head) == 0 {
			continue
		}
		switch last := head[len(head)-1]; {
		case last == '습':
			return true
		case jongseong(last) == jongseongBieup:
			return true
		}
	}
	return false
}

// jongseong은 한글 음절의 종성 번호를 돌려준다. 음절이 아니면 -1이다.
func jongseong(r rune) int {
	if r < 0xAC00 || r > 0xD7A3 {
		return -1
	}
	return int(r-0xAC00) % 28
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
