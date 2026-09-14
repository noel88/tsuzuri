package analyze

import "github.com/noel88/tsuzuri/internal/pack"

// Analysis는 오프라인 분석의 결과다.
//
// 여기에 점수나 정오 판정은 없고, 앞으로도 추가하지 않는다 (스펙 §5.4).
// 참조 번역은 유일한 정답이 아니므로, 참조와 다르다는 이유로 감점하면
// 맞는 답을 틀렸다고 가르치게 된다. 이 구조체는 사실의 나열이다.
type Analysis struct {
	Covered       []string   `json:"covered"`
	Missing       []string   `json:"missing"`
	Flags         []Flag     `json:"flags"` // 미지어 + 의심 형태
	StyleMismatch bool       `json:"style_mismatch"`
	DetectedStyle pack.Style `json:"detected_style,omitempty"`
	RefOnly       []string   `json:"ref_only"`
	AnsOnly       []string   `json:"ans_only"`
	LenRatio      float64    `json:"len_ratio"`
}

// Quiet은 짚을 것이 하나도 없는지 본다.
// 조용하면 화면에서 지적 줄을 통째로 생략한다.
func (a Analysis) Quiet() bool {
	return len(a.Missing) == 0 && len(a.Flags) == 0 && !a.StyleMismatch
}

// Analyzer는 한 방향(=한 사전)에 대한 분석기다.
// 사전 로딩이 비싸므로 재사용한다.
type Analyzer struct {
	dir pack.Direction
	tk  Tokenizer
}

// New는 방향에 맞는 사전을 열어 분석기를 만든다.
// 사전 로딩이 여기서 일어난다 — 포메라에서는 수 초가 걸릴 수 있다.
func New(d pack.Direction) (*Analyzer, error) {
	tk, err := NewTokenizer(d)
	if err != nil {
		return nil, err
	}
	return &Analyzer{dir: d, tk: tk}, nil
}

// Analyze는 답안 하나를 분석한다.
func (a *Analyzer) Analyze(p pack.Problem, answer string) Analysis {
	ansTokens := a.tk.Tokenize(answer)
	refTokens := a.tk.Tokenize(p.Reference)

	covered, missing := Coverage(ansTokens, p.KeyPoints)
	refOnly, ansOnly := VocabDiff(ansTokens, refTokens)

	flags := UnknownTokens(ansTokens)
	flags = append(flags, SuspectForms(ansTokens)...)

	return Analysis{
		Covered:       covered,
		Missing:       missing,
		Flags:         flags,
		StyleMismatch: StyleMismatch(ansTokens, p.Style),
		DetectedStyle: DetectStyle(ansTokens),
		RefOnly:       refOnly,
		AnsOnly:       ansOnly,
		LenRatio:      LenRatio(ansTokens, refTokens),
	}
}
