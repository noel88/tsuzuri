package analyze

import (
	"fmt"
	"testing"

	koDict "github.com/ikawaha/kagome-dict-ko"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// TestProbe는 kagome의 실제 출력을 확인하기 위한 것이다 (M0의 R3).
// 구현을 추측이 아니라 실제 동작에 맞추기 위해 먼저 돌린다.
func TestProbe(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("출력 확인용이다. -v 로 실행한다")
	}
	jaT, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"昨日初めて行ったカフェは静かくて、ずっと座ってました。",
		"長く座っていた",
		"思わなかった",
	} {
		fmt.Printf("\n[JA] %s\n", s)
		for _, tk := range jaT.Tokenize(s) {
			base, ok := tk.BaseForm()
			fmt.Printf("  %-6s cls=%-8v base=%-8s(%v) pos=%v\n",
				tk.Surface, tk.Class, base, ok, tk.POS())
		}
	}

	koT, err := tokenizer.New(koDict.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"어제 처음 간 카페에 갔다",
		"카페가 생각보다 조용해서 오래 앉아 있었다.",
		"앉았습니다",
	} {
		fmt.Printf("\n[KO] %s\n", s)
		for _, tk := range koT.Tokenize(s) {
			fmt.Printf("  %-6s cls=%-8v feat=%v\n", tk.Surface, tk.Class, tk.Features())
		}
	}
}
