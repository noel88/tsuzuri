package analyze

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	koDict "github.com/ikawaha/kagome-dict-ko"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

const shrinkChildEnv = "TSUZURI_SHRINK_CHILD"

// DictShrink는 메모리를 300MB에서 102MB로 줄여 주지만 feature 내용을
// 통째로 버린다. 이 테스트는 그 사실을 고정해, 나중에 메모리를 아끼려고
// DictShrink로 갈아타는 일을 막는다.
//
// 잃는 것:
//   - 일본어: BaseForm() → ("", false). 커버리지 기본형 매칭, 어휘 비교,
//     문체 판정(です/ます)이 전부 Base에 의존한다.
//   - 한국어: features가 [NNG] 하나로 줄어 Expression이 사라진다.
//     koreanBase가 어간을 못 뽑는다.
//
// **반드시 깨끗한 프로세스에서 확인해야 한다.** kagome의 Dict()는
// 자기가 먼저 불리면 shrink 슬롯에도 full 사전을 넣어 둔다:
//
//	func Dict() *dict.Dict {
//	    full.once.Do(func() {
//	        full.dict = loadDict(true)
//	        shrink.once.Do(func() { shrink.dict = full.dict })
//	    })
//	}
//
// 그래서 같은 프로세스에서 Dict()가 한 번이라도 불린 뒤에는
// DictShrink()가 full 사전을 돌려주고, 이 테스트가 거짓 통과한다.
// 자식 프로세스를 새로 띄워 그 오염을 피한다.
func TestDictShrinkLosesFeaturesWeNeed(t *testing.T) {
	if os.Getenv(shrinkChildEnv) == "1" {
		assertShrinkIsUnusable(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestDictShrinkLosesFeaturesWeNeed$", "-test.v")
	cmd.Env = append(os.Environ(), shrinkChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("깨끗한 프로세스에서 실패했다:\n%s", out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("자식 프로세스 결과를 확인할 수 없다:\n%s", out)
	}
}

func assertShrinkIsUnusable(t *testing.T) {
	t.Helper()

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
			t.Fatalf("DictShrink가 Expression을 준다면 이 제약은 사라진 것이다: %v",
				tk.Features())
		}
	}
}
