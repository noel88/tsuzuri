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
	out := RenderMenu(choices, Status{Total: 59, QueueLen: 12}, 92, Cursor{})

	for _, want := range []string{"TSUZURI", "綴", "자 료 실", "41", "ko2ja", "47", "42", "뉴스", "오프라인"} {
		if !strings.Contains(out, want) {
			t.Errorf("초기화면에 %q가 없다:\n%s", want, out)
		}
	}
}

func TestRenderMenuHandlesNoPacks(t *testing.T) {
	out := RenderMenu(nil, Status{}, 92, Cursor{})
	if !strings.Contains(out, "팩이 없습니다") {
		t.Errorf("팩이 없을 때 안내가 있어야 한다:\n%s", out)
	}
}

func TestRenderMenuFitsTerminal(t *testing.T) {
	choices := []Choice{{Key: "41", Label: "ko2ja  N3 일상", Value: "47"}}
	for _, termW := range []int{60, 76, 92, 120} {
		out := RenderMenu(choices, Status{Total: 47}, termW, Cursor{})
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

func TestRenderMenuFitsTerminalWithLongLabels(t *testing.T) {
	// 짧은 라벨만 쓰면 Entry의 넘침을 못 잡는다.
	choices := []Choice{
		{Key: "41", Label: "ko2ja  N1·N2·… 일상생활·해외여행·비즈니스회화·…", Value: "247"},
		{Key: "42", Label: "ja2ko  N1·N2·… 뉴스기사·소설발췌·기술문서·…", Value: "112"},
	}
	for _, termW := range []int{60, 76, 92} {
		out := RenderMenu(choices, Status{Total: 47}, termW, Cursor{})
		for _, line := range strings.Split(out, "\n") {
			if Width(line) > termW {
				t.Errorf("폭 %d에서 넘친다 (%d칸): %q", termW, Width(line), line)
			}
		}
	}
}

func TestRenderMenuShowsReviewCount(t *testing.T) {
	// 복습할 것이 있는지를 들어가 보지 않고도 알아야 한다.
	out := RenderMenu(nil, Status{Due: 7}, 100, Cursor{})
	if !strings.Contains(out, "복습 드릴 (7)") {
		t.Errorf("초기화면에 복습 건수가 없다:\n%s", out)
	}
	// 없으면 (0)을 달지 않는다 — 할 일이 없다는 뜻이 숫자로 보일 필요가 없다.
	out = RenderMenu(nil, Status{}, 100, Cursor{})
	if strings.Contains(out, "복습 드릴 (") {
		t.Errorf("복습할 것이 없는데 건수가 붙었다:\n%s", out)
	}
}

func TestRenderMenuMarksNetworkItems(t *testing.T) {
	// 번호가 바뀌면 이 안내도 같이 바뀌어야 한다. 틀리면 오프라인에서
	// 네트워크가 필요한 항목을 누르게 된다.
	out := RenderMenu(nil, Status{}, 100, Cursor{})
	for _, want := range []string{"8. 팩 받기", "9. 첨삭 받기", "8·9는 네트워크 필요"} {
		if !strings.Contains(out, want) {
			t.Errorf("초기화면에 %q가 없다:\n%s", want, out)
		}
	}
}

func TestRenderMenuTellsHowToGetPacks(t *testing.T) {
	// 팩이 하나도 없을 때 할 수 있는 일을 말해야 한다. 기기 앞에 앉은
	// 사람에게 파일 경로는 쓸모가 없다 — 셸로 나갈 수 있어야 쓰는 말이다.
	out := RenderMenu(nil, Status{}, 100, Cursor{})
	if !strings.Contains(out, "8번으로 받으세요") {
		t.Errorf("팩이 없을 때 안내가 없다:\n%s", out)
	}
}
