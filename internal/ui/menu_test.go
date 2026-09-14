package ui

import (
	"strings"
	"testing"
)

func TestRenderMenuListsPacks(t *testing.T) {
	choices := []Choice{
		{Key: "41", Label: "ko2ja  N3 일상", Value: "47"},
		{Key: "42", Label: "ja2ko  N2 뉴스", Value: "12"},
	}
	out := RenderMenu(choices, Status{Total: 59, QueueLen: 12}, 92)

	for _, want := range []string{"T·S·U·Z·U·R·I", "綴", "자 료 실", "41", "ko2ja", "47", "42", "뉴스", "오프라인"} {
		if !strings.Contains(out, want) {
			t.Errorf("초기화면에 %q가 없다:\n%s", want, out)
		}
	}
}

func TestRenderMenuHandlesNoPacks(t *testing.T) {
	out := RenderMenu(nil, Status{}, 92)
	if !strings.Contains(out, "팩이 없습니다") {
		t.Errorf("팩이 없을 때 안내가 있어야 한다:\n%s", out)
	}
}

func TestRenderMenuFitsTerminal(t *testing.T) {
	choices := []Choice{{Key: "41", Label: "ko2ja  N3 일상", Value: "47"}}
	for _, termW := range []int{60, 76, 92, 120} {
		out := RenderMenu(choices, Status{Total: 47}, termW)
		for _, line := range strings.Split(out, "\n") {
			if Width(line) > termW {
				t.Errorf("폭 %d에서 넘친다 (%d칸): %q", termW, Width(line), line)
			}
		}
	}
}

func TestParseMenuKey(t *testing.T) {
	if k, ok := ParseMenuKey("41"); !ok || k != "41" {
		t.Errorf("ParseMenuKey(41) = %q,%v", k, ok)
	}
	// 「1 드릴 시작」은 첫 팩(=41)을 뜻한다.
	// 이 규약 때문에 main.go는 팩 키를 방향 인덱스가 아니라
	// 세트 인덱스로 매겨야 한다. 방향 인덱스로 매기면 ko2ja 팩이
	// 없을 때 첫 세트가 42가 되어 이 메뉴 항목이 죽는다.
	if k, ok := ParseMenuKey("1"); !ok || k != "41" {
		t.Errorf("「1 드릴 시작」은 첫 팩이어야 한다: %q,%v", k, ok)
	}
	if _, ok := ParseMenuKey("x"); ok {
		t.Error("x는 종료여야 한다")
	}
	if _, ok := ParseMenuKey("q"); ok {
		t.Error("q도 종료여야 한다")
	}
}
