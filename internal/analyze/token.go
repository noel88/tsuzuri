package analyze

import (
	"fmt"
	"strings"

	koDict "github.com/ikawaha/kagome-dict-ko"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"

	"github.com/noel88/tsuzuri/internal/pack"
)

// Token은 사전 종류에 무관한 형태소 표현이다.
// 일본어(IPADIC)와 한국어(mecab-ko-dic)의 feature 구조 차이를
// 전부 이 파일 안에서 흡수한다.
type Token struct {
	Surface string
	Base    string // 기본형. 한국어는 Expression 필드에서 뽑는다.
	POS     string // 품사 대분류 (일본어: 名詞/動詞…, 한국어: NNG/VV+EC…)
	POS1    string // 품사 세분류 (일본어만. 形容動詞語幹 판정에 쓴다)
	Unknown bool   // 사전에 없는 토큰 — 오타의 신호
}

// IsSymbol은 기호·공백인지 본다.
//
// 한국어 사전은 띄어쓰기마다 SP 토큰을 UNKNOWN으로 내놓는다.
// 걸러내지 않으면 미지어 목록이 공백으로 도배된다.
func (t Token) IsSymbol() bool {
	if strings.TrimSpace(t.Surface) == "" {
		return true
	}
	if t.POS == "記号" {
		return true
	}
	// mecab-ko-dic 기호류: SF(마침표) SP(공백) SS SE SO SW SL SH SN
	if len(t.POS) > 0 && t.POS[0] == 'S' {
		switch t.POS {
		case "SF", "SP", "SE", "SSO", "SSC", "SC", "SY", "SL", "SH", "SN", "SW":
			return true
		}
	}
	return false
}

// IsContent는 내용어(명사·동사·형용사·부사)인지 본다.
// 어휘 비교에서 조사·어미·기호를 제외하기 위해 쓴다.
func (t Token) IsContent() bool {
	if t.IsSymbol() {
		return false
	}
	switch t.POS {
	case "名詞", "動詞", "形容詞", "副詞":
		return true
	}
	// 한국어. 복합 태그(VV+EC)는 첫 성분으로 판단한다.
	head := t.POS
	if i := strings.Index(head, "+"); i >= 0 {
		head = head[:i]
	}
	switch {
	case strings.HasPrefix(head, "NN"), // 명사류
		strings.HasPrefix(head, "VV"),  // 동사
		strings.HasPrefix(head, "VA"),  // 형용사
		strings.HasPrefix(head, "VX"),  // 보조용언
		strings.HasPrefix(head, "MAG"), // 일반부사
		head == "XR":                   // 어근 (「조용」해서의 「조용」)
		return true
	}
	return false
}

// Tokenizer는 문장을 형태소로 나눈다.
type Tokenizer interface {
	Tokenize(string) []Token
}

// NewTokenizer는 사용자가 **작문하는 언어**의 사전을 연다.
// 제시문 언어가 아니라는 점에 주의한다.
func NewTokenizer(d pack.Direction) (Tokenizer, error) {
	switch d {
	case pack.KoToJa: // 한국어 제시 → 일본어로 작문
		t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
		if err != nil {
			return nil, err
		}
		return &jaTokenizer{t: t}, nil
	case pack.JaToKo: // 일본어 제시 → 한국어로 작문
		t, err := tokenizer.New(koDict.Dict(), tokenizer.OmitBosEos())
		if err != nil {
			return nil, err
		}
		return &koTokenizer{t: t}, nil
	default:
		return nil, fmt.Errorf("알 수 없는 방향: %q", d)
	}
}

type jaTokenizer struct{ t *tokenizer.Tokenizer }

func (j *jaTokenizer) Tokenize(s string) []Token {
	raw := j.t.Tokenize(s)
	out := make([]Token, 0, len(raw))
	for _, r := range raw {
		tok := Token{
			Surface: r.Surface,
			Base:    r.Surface,
			Unknown: r.Class == tokenizer.UNKNOWN,
		}
		if pos := r.POS(); len(pos) > 0 {
			tok.POS = pos[0]
			if len(pos) > 1 {
				tok.POS1 = pos[1]
			}
		}
		// IPADIC은 BaseForm을 제공한다.
		if b, ok := r.BaseForm(); ok && b != "*" && b != "" {
			tok.Base = b
		}
		if tok.IsSymbol() {
			tok.Unknown = false
		}
		out = append(out, tok)
	}
	return out
}

type koTokenizer struct{ t *tokenizer.Tokenizer }

func (k *koTokenizer) Tokenize(s string) []Token {
	raw := k.t.Tokenize(s)
	out := make([]Token, 0, len(raw))
	for _, r := range raw {
		f := r.Features()
		tok := Token{
			Surface: r.Surface,
			Base:    r.Surface,
			Unknown: r.Class == tokenizer.UNKNOWN,
		}
		if len(f) > 0 {
			tok.POS = f[0]
		}
		tok.Base = koreanBase(r.Surface, f)
		// 띄어쓰기는 SP 토큰으로 나오며 UNKNOWN으로 분류된다.
		// 오타가 아니므로 미지어에서 제외한다.
		if tok.IsSymbol() {
			tok.Unknown = false
		}
		out = append(out, tok)
	}
	return out
}

// koreanBase는 mecab-ko-dic 토큰에서 어간 원형을 뽑는다.
//
// mecab-ko-dic은 BaseForm을 제공하지 않는다. 대신 활용형(Type=="Inflect")일 때
// Expression 필드에 형태소 분해가 들어 있다:
//
//	간    VV+ETM,*,T,간,Inflect,VV,ETM,가/VV/*+ᆫ/ETM/*
//	해서  VV+EC,*,F,해서,Inflect,VV,EC,하/VV/*+아서/EC/*
//	                                    ^^ 어간 원형
//
// 첫 형태소의 "/" 앞부분이 어간이다. 활용형이 아니면 표층이 곧 기본형이다.
//
// 완벽하지 않다. ko-dic이 「갔다」를 「갔다오+ㄴ」으로 분석하는 등
// 오분석이 있다. 이 값은 어휘 비교의 힌트이지 판정 근거가 아니다.
func koreanBase(surface string, f []string) string {
	if len(f) <= koDict.Expression {
		return surface
	}
	if f[koDict.Type] != "Inflect" {
		return surface
	}
	expr := f[koDict.Expression]
	if expr == "" || expr == "*" {
		return surface
	}
	first := expr
	if i := strings.Index(first, "+"); i >= 0 {
		first = first[:i]
	}
	if i := strings.Index(first, "/"); i >= 0 {
		first = first[:i]
	}
	if first == "" {
		return surface
	}
	return first
}
