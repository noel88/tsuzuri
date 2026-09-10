package ui

import (
	"strings"
	"testing"
)

func assertWidths(t *testing.T, lines []string, want int, what string) {
	t.Helper()
	for i, l := range lines {
		if Width(l) != want {
			t.Errorf("%s %d번째 줄 폭 = %d, 기대 %d: %q", what, i, Width(l), want, l)
		}
	}
}

func TestBannerKeepsWidthWithCJK(t *testing.T) {
	got := Banner("綴 / TSUZURI", "한↔일 번역 작문 드릴", 70)
	if len(got) != 3 {
		t.Fatalf("배너는 3줄이어야 한다: %d", len(got))
	}
	assertWidths(t, got, 70, "Banner")
	if !strings.Contains(got[1], "綴") || !strings.Contains(got[1], "드릴") {
		t.Errorf("제목과 부제가 모두 있어야 한다: %q", got[1])
	}
}

func TestFrameKeepsWidth(t *testing.T) {
	got := Frame("p047  ko2ja  N3  일상", "12/47", 76)
	assertWidths(t, got, 76, "Frame")
}

func TestFrameDropsRightWhenTooNarrow(t *testing.T) {
	// 좁으면 오른쪽을 버리고 왼쪽 식별자를 지킨다.
	got := Frame("p047  ko2ja  N3  일상", "12/47", 20)
	assertWidths(t, got, 20, "Frame(narrow)")
	if !strings.Contains(got[1], "p047") {
		t.Errorf("좁아도 문제 번호는 남아야 한다: %q", got[1])
	}
}

func TestBoxWrapsLabel(t *testing.T) {
	got := Box("자 료 실")
	if len(got) != 3 {
		t.Fatalf("상자는 3줄: %d", len(got))
	}
	want := Width("자 료 실") + 4
	assertWidths(t, got, want, "Box")
}

func TestMenuItemHasLabelAndRule(t *testing.T) {
	got := MenuItem("1", "드릴 시작", 22)
	if len(got) != 2 {
		t.Fatalf("항목은 2줄: %d", len(got))
	}
	if !strings.Contains(got[0], "1. 드릴 시작") {
		t.Errorf("항목 줄 = %q", got[0])
	}
	if strings.Count(got[1], "─") != 22 {
		t.Errorf("밑줄 길이 = %d, 기대 22", strings.Count(got[1], "─"))
	}
}

func TestEntryRightAlignsValue(t *testing.T) {
	got := Entry("41", "ko2ja  N3 일상", "47", 40)
	if Width(got) != 40 {
		t.Errorf("폭 = %d, 기대 40: %q", Width(got), got)
	}
	if !strings.HasSuffix(got, "47") {
		t.Errorf("수치가 오른쪽 끝에 와야 한다: %q", got)
	}
}

func TestEntryWithoutValue(t *testing.T) {
	got := Entry("41", "설정", "", 40)
	if strings.TrimSpace(got) != "41. 설정" {
		t.Errorf("Entry = %q", got)
	}
}

func TestTwoColAlignsRightColumn(t *testing.T) {
	left := []string{"짧게", "조금 더 긴 왼쪽 줄"}
	right := []string{"AAA", "BBB"}
	got := TwoCol(left, right, 30, 4)

	// 오른쪽 칼럼의 시작 위치가 두 줄 모두 같아야 한다.
	i0 := Width(got[0]) - Width("AAA")
	i1 := Width(got[1]) - Width("BBB")
	if i0 != i1 {
		t.Errorf("오른쪽 칼럼 시작이 어긋난다: %d vs %d\n%q\n%q", i0, i1, got[0], got[1])
	}
}

func TestTwoColPadsShorterColumn(t *testing.T) {
	got := TwoCol([]string{"A"}, []string{"1", "2", "3"}, 10, 2)
	if len(got) != 3 {
		t.Fatalf("긴 쪽에 맞춰야 한다: %d줄", len(got))
	}
	if !strings.Contains(got[2], "3") {
		t.Errorf("마지막 줄 = %q", got[2])
	}
}

func TestCommandBarShape(t *testing.T) {
	got := CommandBar("주요명령(다음 ⏎, 첨삭 F)  종료(X)", "선택 >>", 70)
	if len(got) != 3 {
		t.Fatalf("명령줄은 3줄: %d", len(got))
	}
	if strings.Count(got[0], "─") != 68 {
		t.Errorf("구분선 길이 = %d, 기대 68", strings.Count(got[0], "─"))
	}
	if !strings.Contains(got[2], "선택 >>") {
		t.Errorf("프롬프트 = %q", got[2])
	}
}

func TestCommandBarWithKeepsStatusAndCommands(t *testing.T) {
	got := CommandBarWith("큐 12건 · 오프라인",
		"주요명령(다음 ⏎, 첨삭 F, 다시 R)  메뉴(M)  종료(X)", "선택 >>", 70)
	if len(got) != 3 {
		t.Fatalf("3줄이어야 한다: %d", len(got))
	}
	if !strings.Contains(got[0], "오프라인") {
		t.Errorf("상태가 구분선에 있어야 한다: %q", got[0])
	}
	if !strings.Contains(got[1], "종료(X)") {
		t.Errorf("명령이 잘리면 안 된다: %q", got[1])
	}
	if Width(got[0]) != 70 {
		t.Errorf("구분선 폭 = %d, 기대 70", Width(got[0]))
	}
}
