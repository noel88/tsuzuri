package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noel88/tsuzuri/internal/config"
)

// SetupFields는 설정 화면의 항목 순서다. 번호가 곧 인덱스+1이다.
var SetupFields = []struct {
	Key   string
	Label string
}{
	{"api_key", "API 키"},
	{"model", "모델"},
	{"level", "기본 레벨"},
	{"topic", "기본 주제"},
	{"pack_size", "팩 크기"},
}

// RenderSetup은 설정 화면이다.
//
// API 키는 마스킹해서 보여준다. 그리고 이 파일이 SD카드에 평문으로
// 남는다는 것을 반드시 알린다 (스펙 §7.5).
func RenderSetup(c config.Config, st Status, termW int) string {
	w := BoxWidth(termW)
	inner := w - 4

	values := map[string]string{
		"api_key":   maskKey(c.APIKey),
		"model":     c.Model,
		"level":     c.Level,
		"topic":     orDash(c.Topic),
		"pack_size": strconv.Itoa(c.PackSize),
	}

	var lines []string
	lines = append(lines, Frame("설 정", "", w)...)
	lines = append(lines, "")
	for i, f := range SetupFields {
		lines = append(lines, Entry(strconv.Itoa(i+1), f.Label, values[f.Key], inner))
	}
	lines = append(lines, "")
	lines = append(lines, "  "+Divider(inner))
	lines = append(lines, wrapIndent(
		"주의: 이 설정은 SD카드에 평문으로 저장됩니다. 기기나 SD를 잃으면 "+
			"키가 노출됩니다. 이 앱 전용 키를 발급하고 사용량 제한을 걸어 두세요. "+
			"환경변수 ANTHROPIC_API_KEY를 쓰면 SD카드에 키를 두지 않아도 됩니다.",
		inner, "  ")...)
	lines = append(lines, "  "+Divider(inner))
	lines = append(lines, "")
	lines = append(lines, CommandBarWith(
		"설정",
		"주요명령(고칠 번호)  메뉴(M)",
		"선택 >>", w)...)

	return Page(lines, termW, w)
}

// maskKey는 키가 설정되어 있음만 보여준다.
func maskKey(k string) string {
	if k == "" {
		return "(없음)"
	}
	if len(k) <= 10 {
		return "sk-ant-…"
	}
	return k[:10] + "…"
}

func orDash(s string) string {
	if s == "" {
		return "(없음)"
	}
	return s
}

// ParseSetupAnswer는 설정 한 항목을 갱신한다.
// 빈 입력은 기존 값을 유지한다 — 실수로 지우는 일을 막는다.
func ParseSetupAnswer(field, input string, c config.Config) (config.Config, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return c, nil
	}
	switch field {
	case "api_key":
		c.APIKey = input
	case "model":
		c.Model = input
	case "level":
		c.Level = input
	case "topic":
		c.Topic = input
	case "pack_size":
		n, err := strconv.Atoi(input)
		if err != nil {
			return c, fmt.Errorf("숫자를 입력하세요: %q", input)
		}
		if n <= 0 {
			return c, fmt.Errorf("1 이상이어야 합니다: %d", n)
		}
		c.PackSize = n
	default:
		return c, fmt.Errorf("알 수 없는 항목: %q", field)
	}
	return c, nil
}
