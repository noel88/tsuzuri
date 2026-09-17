package ui

import (
	"strings"
	"testing"
)

func longPage(bodyLines int) string {
	var b []string
	for i := 0; i < bodyLines; i++ {
		b = append(b, "본문 "+strings.Repeat("가", 3)+" 줄")
	}
	b = append(b, CommandBarWith("상태", "주요명령", "선택 >>", 60)...)
	return strings.Join(b, "\n")
}

func TestScrollLeavesShortPagesAlone(t *testing.T) {
	page := longPage(5)
	got, max := Scroll(page, 36, 0)
	if got != page || max != 0 {
		t.Errorf("짧은 화면을 건드렸다 (max=%d)", max)
	}
}

func TestScrollKeepsCommandBarAtTheBottom(t *testing.T) {
	// 본문이 길다고 무엇을 누를 수 있는지가 화면 밖으로 밀려나면 안 된다.
	got, max := Scroll(longPage(100), 24, 0)
	lines := strings.Split(got, "\n")
	if max == 0 {
		t.Fatal("긴 화면인데 넘길 수 없다")
	}
	if len(lines) > 24 {
		t.Errorf("%d줄을 그렸다. 화면은 24줄이다", len(lines))
	}
	if !strings.Contains(lines[len(lines)-1], "선택 >>") {
		t.Errorf("맨 아래가 명령줄이 아니다: %q", lines[len(lines)-1])
	}
	if !strings.Contains(got, "더 있습니다") {
		t.Error("더 있다는 표시가 없다")
	}
}

func TestScrollStopsAtTheEnd(t *testing.T) {
	page := longPage(100)
	_, max := Scroll(page, 24, 0)

	end, _ := Scroll(page, 24, max)
	if !strings.Contains(end, "여기가 끝입니다") {
		t.Error("끝에 닿은 것을 알려주지 않는다")
	}
	// 더 내려달라고 해도 끝을 넘지 않는다.
	over, _ := Scroll(page, 24, max+50)
	if over != end {
		t.Error("끝을 지나 내려갔다")
	}
}

func TestScrollHintDoesNotUseAmbiguousGlyphs(t *testing.T) {
	// 「↑」는 폭이 애매한 문자다. 터미널이 두 칸으로 그리면 줄이 접힌다.
	got, _ := Scroll(longPage(100), 24, 1)
	for _, r := range got {
		if r == '↑' || r == '↓' || r == '…' {
			t.Errorf("폭이 애매한 문자가 들어갔다: %q", r)
		}
	}
}
