package analyze

import "strings"

// Flag는 "확인해 보라"는 표시다. 오답 판정이 아니다 (스펙 §5.4).
type Flag struct {
	Text string `json:"text"` // 문제가 되는 표층 (「静かく」)
	Why  string `json:"why"`  // 왜 짚었는지
}

// Coverage는 key_points 각 항목이 답안에 나타나는지 확인한다.
//
// 세 가지로 매칭한다:
//  1. 형태소의 기본형 (「思わなかった」→「思う」)
//  2. 형태소의 표층형
//  3. 표층을 이어붙인 문자열에 포함 (「ていた」 같은 연결 표현)
//
// missing이 곧 오답을 뜻하지 않는다. 다른 표현으로 같은 뜻을 썼을 수 있다.
func Coverage(tokens []Token, keyPoints []string) (covered, missing []string) {
	bases := make(map[string]bool, len(tokens))
	surfaces := make(map[string]bool, len(tokens))
	var joined strings.Builder
	for _, t := range tokens {
		bases[t.Base] = true
		surfaces[t.Surface] = true
		joined.WriteString(t.Surface)
	}
	flat := joined.String()

	covered, missing = []string{}, []string{}
	for _, kp := range keyPoints {
		needle := stripTilde(kp)
		if needle == "" {
			continue
		}
		if bases[needle] || surfaces[needle] || strings.Contains(flat, needle) {
			covered = append(covered, kp)
		} else {
			missing = append(missing, kp)
		}
	}
	return covered, missing
}

// stripTilde는 key_point의 물결표를 제거한다. 「〜ていた」→「ていた」
func stripTilde(s string) string {
	for _, t := range []string{"〜", "～", "~"} {
		s = strings.ReplaceAll(s, t, "")
	}
	return strings.TrimSpace(s)
}

// UnknownTokens는 사전에 없는 토큰을 모은다.
//
// 오타의 강한 신호지만 고유명사도 여기 걸리므로 판정이 아니라 확인 요청이다.
// 기호·공백은 제외한다 — ko-dic은 띄어쓰기를 UNKNOWN으로 내놓는다.
func UnknownTokens(tokens []Token) []Flag {
	out := []Flag{}
	for _, t := range tokens {
		if t.Unknown && !t.IsSymbol() {
			out = append(out, Flag{Text: t.Surface, Why: "사전에 없는 형태"})
		}
	}
	return out
}

// SuspectForms는 사전에는 있지만 오용으로 의심되는 형태를 찾는다.
//
// 미지어 탐지만으로는 부족하기 때문에 필요하다. 예를 들어 「静かくて」는
// な형용사를 い형용사처럼 활용한 오류인데, IPADIC은 이것을
// 静か(名詞,形容動詞語幹) + く(動詞,非自立)로 정상 분해해버린다.
// 미지어가 하나도 안 나오므로 품사 나열 패턴으로 잡아야 한다.
func SuspectForms(tokens []Token) []Flag {
	out := []Flag{}
	for i := 0; i+1 < len(tokens); i++ {
		cur, next := tokens[i], tokens[i+1]

		// な형용사 어간 + 「く」 → い형용사 활용을 잘못 적용한 것
		if cur.POS == "名詞" && cur.POS1 == "形容動詞語幹" && next.Surface == "く" {
			out = append(out, Flag{
				Text: cur.Surface + next.Surface,
				Why:  "な형용사는 「" + cur.Surface + "で」로 활용한다",
			})
		}
	}
	return out
}
