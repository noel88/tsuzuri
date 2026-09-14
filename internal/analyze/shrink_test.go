package analyze

import (
	"testing"

	koDict "github.com/ikawaha/kagome-dict-ko"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// DictShrink는 메모리를 300MB에서 102MB로 줄여 주지만 feature 내용을
// 통째로 버린다. 이 테스트는 그 사실을 고정해, 나중에 메모리를 아끼려고
// DictShrink로 갈아타는 일을 막는다.
//
// 잃는 것:
//   - 일본어: BaseForm() → ("", false). 커버리지 기본형 매칭, 어휘 비교,
//     문체 판정(です/ます)이 전부 Base에 의존한다.
//   - 한국어: features가 [NNG] 하나로 줄어 Expression이 사라진다.
//     koreanBase가 어간을 못 뽑는다.
func TestDictShrinkLosesFeaturesWeNeed(t *testing.T) {
	jt, err := tokenizer.New(ipa.DictShrink(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range jt.Tokenize("長く座っていた") {
		if b, ok := tk.BaseForm(); ok && b != "" {
			t.Fatalf("DictShrink가 BaseForm을 준다면 이 제약은 사라진 것이다: %q → %q\n"+
				"main.go의 방향 전환 재실행 로직을 재검토하라", tk.Surface, b)
		}
	}

	kt, err := tokenizer.New(koDict.DictShrink(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range kt.Tokenize("조용해서") {
		if len(tk.Features()) > koDict.Expression {
			t.Fatalf("DictShrink가 Expression을 준다면 이 제약은 사라진 것이다: %v", tk.Features())
		}
	}
}
