package ui

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func br(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestReadLineStripsNewline(t *testing.T) {
	got, eof, err := ReadLine(br("昨日カフェに行った\n남은 줄\n"))
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if eof {
		t.Error("아직 입력이 남아 있다")
	}
	if got != "昨日カフェに行った" {
		t.Errorf("ReadLine = %q", got)
	}
}

func TestReadLineReportsEOF(t *testing.T) {
	r := br("한 줄뿐")
	got, eof, err := ReadLine(r)
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "한 줄뿐" {
		t.Errorf("EOF 직전 내용도 유효해야 한다: %q", got)
	}
	if !eof {
		t.Error("EOF를 보고해야 한다")
	}
}

func TestReadLineDistinguishesBlankFromEOF(t *testing.T) {
	// 빈 줄과 입력 종료를 구분하지 못하면 드릴 루프가 무한히 돈다.
	r := br("\n")
	got, eof, err := ReadLine(r)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" || eof {
		t.Errorf("빈 줄이어야 한다: got=%q eof=%v", got, eof)
	}
	_, eof2, _ := ReadLine(r)
	if !eof2 {
		t.Error("그 다음이 EOF여야 한다")
	}
}

func TestParseCommand(t *testing.T) {
	cases := map[string]Command{
		"":   CmdNext,
		"f":  CmdPriority,
		"F":  CmdPriority,
		" f": CmdPriority,
		"r":  CmdRetry,
		"m":  CmdMenu,
		"x":  CmdQuit,
		"q":  CmdQuit,
		"zz": CmdNext,
	}
	for in, want := range cases {
		if got := ParseCommand(in); got != want {
			t.Errorf("ParseCommand(%q) = %v, 기대 %v", in, got, want)
		}
	}
}

func TestIsEditorRequest(t *testing.T) {
	if !IsEditorRequest(":e") || !IsEditorRequest(" :e ") {
		t.Error(":e는 에디터 요청이어야 한다")
	}
	if IsEditorRequest("일반 답안") || IsEditorRequest(":ee") {
		t.Error("일반 입력을 에디터 요청으로 보면 안 된다")
	}
}

func TestReadFromEditorReturnsFileContent(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-editor.sh")
	body := "#!/bin/sh\nprintf '%s' '카페에서 오래 앉아 있었다' > \"$1\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFromEditor(script, "")
	if err != nil {
		t.Fatalf("ReadFromEditor: %v", err)
	}
	if got != "카페에서 오래 앉아 있었다" {
		t.Errorf("ReadFromEditor = %q", got)
	}
}

func TestReadFromEditorSeedsInitialContent(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "noop-editor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFromEditor(script, "초기 내용")
	if err != nil {
		t.Fatalf("ReadFromEditor: %v", err)
	}
	if got != "초기 내용" {
		t.Errorf("초기 내용이 보존되어야 한다: %q", got)
	}
}

func TestIsCancel(t *testing.T) {
	for _, s := range []string{":q", ":quit", " :Q ", ":cancel"} {
		if !IsCancel(s) {
			t.Errorf("%q는 취소여야 한다", s)
		}
	}
	// 값을 묻는 자리에서는 m·x·q도 값일 수 있다(주제가 "x"일 수 있다).
	for _, s := range []string{"m", "x", "q", "", "N3", "일상"} {
		if IsCancel(s) {
			t.Errorf("%q를 취소로 보면 안 된다", s)
		}
	}
}

// mozc가 히라가나 모드면 x를 쳐도 전각 「ｘ」가 확정된다. 그대로 두면
// 종료하려던 입력이 답안으로 저장되고 첨삭 대기열에까지 들어간다.
func TestParseCommandAcceptsFullWidthLetters(t *testing.T) {
	cases := map[string]Command{
		"ｘ": CmdQuit, "Ｘ": CmdQuit, "ｑ": CmdQuit,
		"ｍ": CmdMenu, "ｆ": CmdPriority, "ｒ": CmdRetry,
	}
	for in, want := range cases {
		if got := ParseCommand(in); got != want {
			t.Errorf("ParseCommand(%q) = %v, 기대 %v", in, got, want)
		}
	}
}

func TestFullWidthCancelAndEditor(t *testing.T) {
	if !IsCancel("：ｑ") {
		t.Error("전각 :q도 취소여야 한다")
	}
	if !IsEditorRequest("：ｅ") {
		t.Error("전각 :e도 에디터 요청이어야 한다")
	}
}

func TestReadLineAcceptsCarriageReturn(t *testing.T) {
	// 화살표를 읽느라 잠깐 raw mode였던 동안 버퍼에 들어온 바이트는
	// 터미널의 \r → \n 변환을 거치지 않는다. 그것을 못 알아보면 이미
	// 들어와 있는 줄을 영영 못 읽는다 — 답안이 통째로 사라졌다.
	r := bufio.NewReader(strings.NewReader("こたえ\r"))
	got, eof, err := ReadLine(r)
	if err != nil {
		t.Fatal(err)
	}
	if got != "こたえ" || eof {
		t.Errorf("ReadLine = %q, eof=%v", got, eof)
	}
}

func TestReadLineSplitsBurstIntoLines(t *testing.T) {
	// 사람이 빠르게 치면 여러 줄이 한 덩어리로 온다. 섞여 있어도 각각
	// 제 줄로 끊겨야 한다.
	r := bufio.NewReader(strings.NewReader("첫째\r둘째\n셋째\r\n넷째\r"))
	for _, want := range []string{"첫째", "둘째", "셋째", "넷째"} {
		got, _, err := ReadLine(r)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("ReadLine = %q, 기대 %q", got, want)
		}
	}
}

func TestReadLineKeepsWhatCameBeforeEOF(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("끝줄"))
	got, eof, err := ReadLine(r)
	if err != nil {
		t.Fatal(err)
	}
	if got != "끝줄" || !eof {
		t.Errorf("ReadLine = %q, eof=%v", got, eof)
	}
}
