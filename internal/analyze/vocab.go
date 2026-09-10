package analyze

// VocabDiff는 답안과 참조의 내용어를 비교한다.
// 조사·어미·기호는 제외한다 — 규칙으로 판정할 수 없는 영역이다.
//
// refOnly는 "참조에는 있는데 내 답에는 없는 것", ansOnly는 그 반대다.
// 어느 쪽도 오류를 뜻하지 않는다. 번역은 정답이 여럿이다 (스펙 §5.4).
func VocabDiff(answer, reference []Token) (refOnly, ansOnly []string) {
	ansSet := contentSet(answer)
	refSet := contentSet(reference)

	refOnly = []string{}
	for _, w := range contentWords(reference) {
		if !ansSet[w] {
			refOnly = append(refOnly, w)
		}
	}
	ansOnly = []string{}
	for _, w := range contentWords(answer) {
		if !refSet[w] {
			ansOnly = append(ansOnly, w)
		}
	}
	return refOnly, ansOnly
}

// contentWords는 내용어의 기본형을 등장 순서대로, 중복 없이 돌려준다.
func contentWords(ts []Token) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range ts {
		if !t.IsContent() || t.Base == "" {
			continue
		}
		if seen[t.Base] {
			continue
		}
		seen[t.Base] = true
		out = append(out, t.Base)
	}
	return out
}

func contentSet(ts []Token) map[string]bool {
	set := map[string]bool{}
	for _, w := range contentWords(ts) {
		set[w] = true
	}
	return set
}

// LenRatio는 참조 대비 답안의 형태소 개수 비율이다.
// 기호는 세지 않는다. 1.0에서 크게 벗어나면 지나치게 길거나 짧다는 신호다.
func LenRatio(answer, reference []Token) float64 {
	r := countMeaningful(reference)
	if r == 0 {
		return 0
	}
	return float64(countMeaningful(answer)) / float64(r)
}

func countMeaningful(ts []Token) int {
	n := 0
	for _, t := range ts {
		if !t.IsSymbol() {
			n++
		}
	}
	return n
}
