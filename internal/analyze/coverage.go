package analyze

import "strings"

// Flag는 "확인해 보라"는 표시다. 오답 판정이 아니다 (스펙 §5.4).
type Flag struct {
	Text string `json:"text"` // 문제가 되는 표층 (「静かく」)
	Why  string `json:"why"`  // 왜 짚었는지
}

// Coverage는 key_points 각 항목이 답안에 나타나는지 확인한다.
//
// 네 가지로 매칭한다:
//  1. 형태소의 기본형 (「思わなかった」→「思う」)
//  2. 형태소의 표층형
//  3. 표층을 이어붙인 문자열에 포함 (「ていた」 같은 연결 표현)
//  4. 띄어쓰기를 무시한 비교
//
// 4번이 필요한 이유: 한국어 보조용언은 붙여 써도 맞다(한글 맞춤법 제47항).
// 「앉아 있」을 요구하는 문제에 「앉아있었다」라고 답하면 맞는 답인데,
// 띄어쓰기까지 따지면 빠졌다고 나온다. 사전형으로 적힌 핵심 표현도
// 어간만으로 비교해서 「가져오다」와 「가져왔습니다」를 맞춘다.
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
	tight := squeeze(flat)

	covered, missing = []string{}, []string{}
	for _, kp := range keyPoints {
		needle := stripTilde(kp)
		if needle == "" {
			continue
		}
		if matches(needle, bases, surfaces, flat, tight) {
			covered = append(covered, kp)
		} else {
			missing = append(missing, kp)
		}
	}
	return covered, missing
}

func matches(needle string, bases, surfaces map[string]bool, flat, tight string) bool {
	if bases[needle] || surfaces[needle] || strings.Contains(flat, needle) {
		return true
	}
	// 띄어쓰기를 무시하고 다시 본다.
	if n := squeeze(needle); n != "" && strings.Contains(tight, n) {
		return true
	}
	// 사전형으로 적힌 핵심 표현은 어미를 떼고 어간으로 본다.
	if stem := dictionaryStem(needle); stem != "" {
		if bases[stem] {
			return true
		}
		// 글자로 찾는 것은 어간이 두 글자 이상일 때만 한다.
		//
		// 한 글자 어간은 아무 문장에나 걸린다. 「사다」의 「사」가 「사진」에,
		// 「오다」의 「오」가 「오늘」에 걸려서, 쓰지도 않은 표현을 썼다고
		// 알려 줬다. 형태소로 찾은 것(bases)은 그런 일이 없다.
		if len([]rune(stem)) >= 2 && strings.Contains(tight, squeeze(stem)) {
			return true
		}
	}
	return false
}

// squeeze는 공백을 모두 없앤다.
func squeeze(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// dictionaryStem은 사전형 표현에서 어간을 뽑는다.
//
// 한국어 동사·형용사는 「가져오다」처럼 「다」로 적히는데, koreanBase는
// 어간만(「가져오」) 돌려주므로 그대로는 만나지 못한다. 일본어에는 이런
// 꼴이 없으므로 한글일 때만 적용한다.
func dictionaryStem(s string) string {
	if !strings.HasSuffix(s, "다") {
		return ""
	}
	stem := strings.TrimSuffix(s, "다")
	if stem == "" || !isHangul(stem) {
		return ""
	}
	return stem
}

func isHangul(s string) bool {
	for _, r := range s {
		if r < 0xAC00 || r > 0xD7A3 {
			return false
		}
	}
	return true
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
