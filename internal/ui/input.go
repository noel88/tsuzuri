package ui

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Command는 결과 화면에서의 다음 동작이다.
type Command int

const (
	CmdNext     Command = iota // Enter — 다음 문제
	CmdPriority                // F — 첨삭 우선 처리 표시
	CmdRetry                   // R — 같은 문제 다시 풀기
	CmdMark                    // B — 복습 목록에 넣기
	CmdMenu                    // M — 메뉴로
	CmdQuit                    // X — 종료
	CmdEdit                    // :e — 에디터로 긴 답 쓰기
	CmdStay                    // 하던 일로 돌아가기
)

// ReadLine은 한 줄을 읽는다.
//
// raw mode를 쓰지 않는 것이 핵심이다 (스펙 §6.1).
// 터미널의 정상 입력 경로를 그대로 써야 mozc IME가 동작한다.
//
// 두 번째 반환값은 입력이 끝났는지(EOF) 여부다.
// 빈 줄과 입력 종료를 구분해야 드릴 루프가 무한히 돌지 않는다.
func ReadLine(r *bufio.Reader) (string, bool, error) {
	var b strings.Builder
	for {
		c, err := r.ReadByte()
		if err == io.EOF {
			// EOF 직전에 읽은 내용은 유효하다.
			return b.String(), true, nil
		}
		if err != nil {
			return "", false, err
		}
		switch c {
		case '\n':
			return b.String(), false, nil
		case '\r':
			// 줄 끝을 \n 하나로만 보면 안 된다.
			//
			// 터미널이 Enter의 \r을 \n으로 바꿔 주는 것은 보통 모드일
			// 때뿐이다. 화살표를 읽느라 잠깐 raw mode였던 동안 버퍼에
			// 들어온 바이트는 그 변환을 거치지 않아 \r 그대로 남는다.
			// 그것을 못 알아보면 이미 들어와 있는 줄을 영영 못 읽고
			// 다음 입력을 기다린다 — 초기화면에서 Enter를 누르고 곧바로
			// 답을 쳐 넣으면 그 답안이 통째로 사라졌다.
			if r.Buffered() > 0 {
				if p, _ := r.Peek(1); len(p) == 1 && p[0] == '\n' {
					r.Discard(1)
				}
			}
			return b.String(), false, nil
		default:
			b.WriteByte(c)
		}
	}
}

// ParseCommand는 결과 화면의 입력을 해석한다.
// 모르는 입력은 다음 문제로 넘어간다 — 흐름을 끊지 않는다.
func ParseCommand(s string) Command {
	switch strings.ToLower(normalizeCommand(s)) {
	case "f":
		return CmdPriority
	case "r":
		return CmdRetry
	case "b":
		return CmdMark
	case "m":
		return CmdMenu
	case "x", "q":
		return CmdQuit
	default:
		return CmdNext
	}
}

// normalizeCommand는 전각 영문자를 반각으로 바꾼다.
//
// mozc가 히라가나 모드면 x를 쳐도 전각 「ｘ」가 확정된다. 그대로 두면
// 명령으로 인식되지 않고, 답안 자리에서는 그 글자가 답안으로 저장되어
// 첨삭 대기열에까지 들어간다.
func normalizeCommand(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'Ａ' && r <= 'Ｚ':
			b.WriteRune(r - 'Ａ' + 'A')
		case r >= 'ａ' && r <= 'ｚ':
			b.WriteRune(r - 'ａ' + 'a')
		case r == '：':
			b.WriteRune(':')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsCancel은 지금 하던 일을 그만두겠다는 입력인지 본다.
//
// 값을 묻는 프롬프트에서는 m이나 x도 값으로 읽혀야 하므로(주제가 "x"일 수
// 있다) 콜론을 붙인 :q 와 :quit 만 취소로 본다. 답안 자리의 :e 와 같은 꼴이다.
func IsCancel(s string) bool {
	switch strings.ToLower(normalizeCommand(s)) {
	case ":q", ":quit", ":cancel":
		return true
	}
	return false
}

// IsEditorRequest는 답안 자리에 ":e"가 입력됐는지 본다.
func IsEditorRequest(s string) bool {
	return normalizeCommand(s) == ":e"
}

// ReadFromEditor는 임시 파일을 에디터로 열고, 저장된 내용을 돌려준다.
// 긴 답안을 쓸 때 쓴다. editor가 비면 $EDITOR, 그것도 없으면 nano.
func ReadFromEditor(editor, initial string) (string, error) {
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "nano"
	}

	dir, err := os.MkdirTemp("", "tsuzuri")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "answer.txt")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		return "", err
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	runErr := cmd.Run()

	// 편집기가 0이 아닌 코드로 끝나도 파일부터 읽는다.
	//
	// vi의 :cq 처럼 일부러 실패로 끝내는 길이 있고, 편집기가 신호로 죽기도
	// 한다. 그때 저장된 것을 안 읽으면 문단 하나를 통째로 다시 써야 한다.
	// 쓴 것이 있으면 그것이 답이다.
	b, err := os.ReadFile(path)
	if err == nil {
		if text := strings.TrimSpace(string(b)); text != "" {
			return text, nil
		}
	}
	if runErr != nil {
		return "", runErr
	}
	if err != nil {
		return "", err
	}
	return "", nil
}
