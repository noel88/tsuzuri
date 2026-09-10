package ui

import (
	"fmt"
	"strings"
	"testing"
)

// TestDemoScreen은 검증이 아니라 화면을 눈으로 확인하기 위한 것이다.
// 확정된 레이아웃(PC통신 스타일, 번호 이동)을 코드로 남겨둔다.
//
//	go test ./internal/ui/ -run TestDemoScreen -v
func TestDemoScreen(t *testing.T) {
	const termW, w = 92, 76

	// ── 초기화면 ──
	var menu []string
	menu = append(menu, Banner("綴 / T·S·U·Z·U·R·I", "한↔일 번역 작문 드릴", w)...)
	menu = append(menu, "", "      팩 2개 · 남은 문제 59 · 첨삭 큐 12건 · 오프라인", "")

	var left []string
	left = append(left, MenuItem("1", "드릴 시작", 22)...)
	left = append(left, MenuItem("2", "이어하기", 22)...)
	left = append(left, MenuItem("3", "팩 받기", 22)...)
	left = append(left, MenuItem("4", "설정", 22)...)

	var right []string
	right = append(right, Box("자 료 실")...)
	right = append(right, Entry("41", "ko2ja  N3 일상", "47", 38))
	right = append(right, Entry("42", "ja2ko  N2 뉴스", "12", 38))
	right = append(right, "")
	right = append(right, Box("복 습")...)
	right = append(right, Entry("51", "첨삭 대기", "12", 38))
	right = append(right, Entry("52", "받은 첨삭", "34", 38))

	menu = append(menu, TwoCol(left, right, 30, 4)...)
	menu = append(menu, "")
	menu = append(menu, CommandBar("주요명령(드릴 D, 복습 R)  이동(번호)  종료(X)", "선택(도움말[H]) >>", w)...)

	// ── 드릴 화면 ──
	var drill []string
	drill = append(drill, Frame("p047   ko2ja   N3   일상", "12/47", w)...)
	drill = append(drill, "")
	drill = append(drill, "  어제 처음 간 카페가 생각보다 조용해서 오래 앉아 있었다.")
	drill = append(drill, "")
	drill = append(drill, "  나  : 昨日初めて行ったカフェは静かくて、ずっと座ってました。")
	drill = append(drill, "  참조: 昨日初めて行ったカフェが思ったより静かで、長く座っていた。")
	drill = append(drill, "")
	drill = append(drill, "  "+Divider(w-4))
	drill = append(drill, "  ✓ 「初めて」   ✓ 「ていた」   ✗ 「思ったより」 빠짐")
	drill = append(drill, "  ⚠ 「静かく」   사전에 없는 형태")
	drill = append(drill, "  ⚠ 문체        답안 정중체 / 문제 보통체")
	drill = append(drill, "  "+Divider(w-4))
	drill = append(drill, "")
	drill = append(drill, CommandBar("주요명령(다음 ⏎, 첨삭 F, 다시 R)  메뉴(M)  종료(X)", "선택 >>", w)...)

	fmt.Println("\n눈금 (92칸):")
	fmt.Println(strings.Repeat("1234567890", 9) + "12")
	fmt.Println("\n===== 초기화면 =====")
	fmt.Print(Join(Center(menu, termW, w)))
	fmt.Println("\n===== 드릴 화면 =====")
	fmt.Print(Join(Center(drill, termW, w)))
}
