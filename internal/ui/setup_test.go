package ui

import (
	"strings"
	"testing"

	"github.com/noel88/tsuzuri/internal/config"
)

func TestRenderSetupWarnsAboutKeyOnSDCard(t *testing.T) {
	// 스펙 §7.5. SD카드에 평문 키가 남는다는 것을 반드시 알린다.
	out := RenderSetup(config.Default(), Status{}, 92)
	if !strings.Contains(out, "SD") {
		t.Errorf("SD카드 보관 위험을 알려야 한다:\n%s", out)
	}
	if !strings.Contains(out, "전용") || !strings.Contains(out, "제한") {
		t.Errorf("전용 키·사용량 제한을 권해야 한다:\n%s", out)
	}
	if !strings.Contains(out, "ANTHROPIC_API_KEY") {
		t.Errorf("환경변수 대안을 알려야 한다:\n%s", out)
	}
}

func TestRenderSetupMasksExistingKey(t *testing.T) {
	c := config.Config{APIKey: "sk-ant-api03-verysecretvalue", Model: "claude-opus-5", PackSize: 50}
	out := RenderSetup(c, Status{}, 92)
	if strings.Contains(out, "verysecretvalue") {
		t.Errorf("키를 화면에 그대로 띄우면 안 된다:\n%s", out)
	}
	if !strings.Contains(out, "sk-ant") {
		t.Errorf("설정되어 있음은 보여야 한다:\n%s", out)
	}
}

func TestRenderSetupShowsMissingKey(t *testing.T) {
	out := RenderSetup(config.Config{Model: "claude-opus-5", PackSize: 50}, Status{}, 92)
	if !strings.Contains(out, "(없음)") {
		t.Errorf("키가 없음을 알려야 한다:\n%s", out)
	}
}

func TestRenderSetupFitsTerminal(t *testing.T) {
	c := config.Config{APIKey: "sk-ant-api03-verysecret", Model: "claude-opus-5", Level: "N3", PackSize: 50}
	for _, termW := range []int{60, 76, 92} {
		out := RenderSetup(c, Status{}, termW)
		for _, line := range strings.Split(out, "\n") {
			if Width(line) > termW {
				t.Errorf("폭 %d에서 넘친다 (%d칸): %q", termW, Width(line), line)
			}
		}
	}
}

func TestParseSetupAnswerUpdatesFields(t *testing.T) {
	c := config.Default()
	c, err := ParseSetupAnswer("level", "N2", c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Level != "N2" {
		t.Errorf("Level = %q", c.Level)
	}
	c, err = ParseSetupAnswer("pack_size", "30", c)
	if err != nil {
		t.Fatal(err)
	}
	if c.PackSize != 30 {
		t.Errorf("PackSize = %d", c.PackSize)
	}
}

func TestParseSetupAnswerRejectsBadPackSize(t *testing.T) {
	if _, err := ParseSetupAnswer("pack_size", "영", config.Default()); err == nil {
		t.Error("숫자가 아니면 오류여야 한다")
	}
	if _, err := ParseSetupAnswer("pack_size", "0", config.Default()); err == nil {
		t.Error("0은 오류여야 한다")
	}
}

func TestParseSetupAnswerKeepsValueOnBlank(t *testing.T) {
	c := config.Config{Level: "N3", PackSize: 50}
	got, err := ParseSetupAnswer("level", "", c)
	if err != nil {
		t.Fatal(err)
	}
	if got.Level != "N3" {
		t.Errorf("빈 입력은 기존 값을 유지해야 한다: %q", got.Level)
	}
}

func TestParseSetupAnswerRejectsUnknownField(t *testing.T) {
	if _, err := ParseSetupAnswer("없는항목", "값", config.Default()); err == nil {
		t.Error("모르는 항목은 오류여야 한다")
	}
}

func TestSetupFieldsMatchRenderedNumbers(t *testing.T) {
	// 화면의 번호와 SetupFields 인덱스가 어긋나면 엉뚱한 값이 바뀐다.
	out := RenderSetup(config.Default(), Status{}, 92)
	for i, f := range SetupFields {
		if !strings.Contains(out, f.Label) {
			t.Errorf("%d번 항목 %q가 화면에 없다", i+1, f.Label)
		}
	}
}
