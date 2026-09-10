package analyze

import (
	"testing"

	"github.com/noel88/tsuzuri/internal/pack"
)

func TestDetectStyleJapaneseViaTokenizer(t *testing.T) {
	tk := jaTok(t)
	cases := []struct {
		text string
		want pack.Style
	}{
		{"昨日カフェに行きました。", pack.StylePolite},
		{"静かでした。", pack.StylePolite},
		{"昨日カフェに行った。", pack.StylePlain},
		{"長く座っていた。", pack.StylePlain},
	}
	for _, c := range cases {
		if got := DetectStyle(tk.Tokenize(c.text)); got != c.want {
			t.Errorf("DetectStyle(%q) = %q, 기대 %q", c.text, got, c.want)
		}
	}
}

func TestDetectStyleKoreanViaTokenizer(t *testing.T) {
	tk := koTok(t)
	cases := []struct {
		text string
		want pack.Style
	}{
		{"어제 카페에 앉았습니다.", pack.StylePolite},
		{"어제 카페에 앉았어요.", pack.StylePolite},
		// 「있었다」의 「다」는 ko-dic에서 EF가 아니라 EC로 태깅된다.
		// EF만 보면 이 문장을 놓친다.
		{"오래 앉아 있었다.", pack.StylePlain},
	}
	for _, c := range cases {
		if got := DetectStyle(tk.Tokenize(c.text)); got != c.want {
			t.Errorf("DetectStyle(%q) = %q, 기대 %q (토큰 %v)",
				c.text, got, c.want, surfaces(tk.Tokenize(c.text)))
		}
	}
}

func TestDetectStyleUndetectable(t *testing.T) {
	tokens := []Token{{Surface: "カフェ", Base: "カフェ", POS: "名詞"}}
	if got := DetectStyle(tokens); got != "" {
		t.Errorf("판단 불가여야 한다: %q", got)
	}
}

func TestStyleMismatchSilentWhenUndetectable(t *testing.T) {
	tokens := []Token{{Surface: "カフェ", Base: "カフェ", POS: "名詞"}}
	if StyleMismatch(tokens, pack.StylePlain) {
		t.Error("판단 불가일 때는 불일치로 보고하면 안 된다")
	}
}

func TestStyleMismatchDetectsConflict(t *testing.T) {
	tokens := jaTok(t).Tokenize("座ってました。")
	if !StyleMismatch(tokens, pack.StylePlain) {
		t.Errorf("정중체 답안 + 보통체 요구 = 불일치 (detected=%q)", DetectStyle(tokens))
	}
	if StyleMismatch(tokens, pack.StylePolite) {
		t.Error("정중체 답안 + 정중체 요구 = 일치")
	}
}

func TestStyleMismatchSilentWhenWantEmpty(t *testing.T) {
	tokens := jaTok(t).Tokenize("座ってました。")
	if StyleMismatch(tokens, "") {
		t.Error("문제가 문체를 요구하지 않으면 짚지 않는다")
	}
}
