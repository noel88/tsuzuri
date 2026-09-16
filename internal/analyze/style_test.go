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

// 리뷰가 찾아낸 오판들. 맞는 답에 문체 경고가 뜨면 학습을 방해한다.
func TestDetectStyleDoesNotMistakeAttributiveNaForPlain(t *testing.T) {
	tk := jaTok(t)
	cases := []struct {
		text string
		want pack.Style
	}{
		// IPADIC은 연체형 「な」와 중지형 「で」에도 기본형 「だ」를 준다.
		// 그것을 보통체 표지로 세면 정중체 문장이 보통체로 판정된다.
		{"雨が降りそうなので傘を持ってきました。", pack.StylePolite},
		{"静かなカフェで本を読みました。", pack.StylePolite},
		{"昨日は静かで、長く座っていました。", pack.StylePolite},
		// 보통체는 그대로 보통체여야 한다.
		{"昨日カフェに行った。", pack.StylePlain},
		{"ここは静かだ。", pack.StylePlain},
		{"静かなカフェで本を読んだ。", pack.StylePlain},
	}
	for _, c := range cases {
		if got := DetectStyle(tk.Tokenize(c.text)); got != c.want {
			t.Errorf("DetectStyle(%q) = %q, 기대 %q", c.text, got, c.want)
		}
	}
}

func TestDetectStyleKoreanFusedAndQuestionEndings(t *testing.T) {
	tk := koTok(t)
	cases := []struct {
		text string
		want pack.Style
	}{
		// 「습니까」는 EF가 아니라 EC로 태깅된다.
		{"어디에 갔습니까?", pack.StylePolite},
		// 어간과 어미가 붙은 토큰은 VCP+EF, VV+EC 같은 복합 태그를 받는다.
		{"저는 학생입니다.", pack.StylePolite},
		{"매일 운동합니다.", pack.StylePolite},
		{"카페에 갔어요.", pack.StylePolite},
		{"매일 간다.", pack.StylePlain},
		{"오래 앉아 있었다.", pack.StylePlain},
	}
	for _, c := range cases {
		if got := DetectStyle(tk.Tokenize(c.text)); got != c.want {
			t.Errorf("DetectStyle(%q) = %q, 기대 %q (토큰 %v)",
				c.text, got, c.want, surfaces(tk.Tokenize(c.text)))
		}
	}
}

// 배포되는 샘플 팩의 모범답안을 그대로 넣으면 아무 지적도 없어야 한다.
func TestShippedSamplesAreQuietWhenAnsweredWithTheirReference(t *testing.T) {
	for _, dir := range []pack.Direction{pack.KoToJa, pack.JaToKo} {
		az, err := New(dir)
		if err != nil {
			t.Fatal(err)
		}
		ps, _, err := pack.LoadDir("../../testdata/packs", dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) == 0 {
			t.Fatalf("%s 샘플이 없다", dir)
		}
		for _, p := range ps {
			got := az.Analyze(p, p.Reference)
			if got.StyleMismatch {
				t.Errorf("%s: 모범답안에 문체 경고가 뜬다 (요구 %q / 판정 %q)",
					p.ID, p.Style, got.DetectedStyle)
			}
			if len(got.Missing) > 0 {
				t.Errorf("%s: 모범답안인데 빠진 표현이 있다: %v", p.ID, got.Missing)
			}
			if len(got.Flags) > 0 {
				t.Errorf("%s: 모범답안에 지적이 있다: %+v", p.ID, got.Flags)
			}
		}
	}
}
