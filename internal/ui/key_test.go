package ui

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// keysFrom은 주어진 바이트에서 읽히는 키를 순서대로 모으고,
// 그 뒤에 한 줄 읽기로 무엇이 남는지 돌려준다.
//
// raw mode를 걸 수 없는 환경이므로(테스트는 터미널이 아니다) 상태 기계만
// 본다. 실제 터미널 동작은 pty로 따로 확인한다.
func keysFrom(t *testing.T, in string, n int) ([]KeyPress, string) {
	t.Helper()
	r := bufio.NewReader(strings.NewReader(in))
	kr := NewKeyReader(r, os.Stdin)

	var got []KeyPress
	for i := 0; i < n; i++ {
		k, err := kr.readNoRaw()
		if err != nil {
			break
		}
		got = append(got, k)
	}
	line, _, _ := ReadLine(r)
	return got, line
}

func TestKeyReaderSwallowsUnknownSequences(t *testing.T) {
	// Delete(ESC [ 3 ~)를 남겨 두면 그 바이트가 답안의 첫 글자가 된다.
	// 「[3~」로 시작하는 답안이 저장되고 첨삭 요청으로 나가 값을 치렀다.
	//
	// 열을 삼킨 뒤에는 그 다음 키가 나온다 — 여기서는 「답」이다.
	got, rest := keysFrom(t, "\x1b[3~답안입니다\r", 1)
	if len(got) != 1 || got[0].Key != KeyRune || got[0].Rune != '답' {
		t.Fatalf("열을 삼킨 뒤 다음 키가 안 나왔다: %+v", got)
	}
	if rest != "안입니다" {
		t.Errorf("남은 것 = %q — 이스케이프 바이트가 섞였다", rest)
	}
}

func TestKeyReaderHandlesSplitSequences(t *testing.T) {
	// 터미널이 ESC 와 나머지를 나눠 건네주는 일이 있다. 한 덩어리로 올
	// 때만 처리하면 쪼개진 경우에 그대로 샌다 — 실기에서 그랬다.
	for _, in := range []string{"\x1b|[3~|답안", "\x1b|[A|답안", "\x1b|OP|답안"} {
		parts := strings.Split(in, "|")
		pr, pw := io.Pipe()
		r := bufio.NewReader(pr)
		kr := NewKeyReader(r, os.Stdin)

		go func() {
			for _, p := range parts {
				pw.Write([]byte(p))
				time.Sleep(20 * time.Millisecond)
			}
			pw.Write([]byte("\r"))
			pw.Close()
		}()

		// 조각이 나뉘어 와도 열을 다 삼킨 뒤에야 다음 키가 나온다.
		var rest string
		for i := 0; i < 5; i++ {
			k, err := kr.readNoRaw()
			if err != nil {
				break
			}
			if k.Key == KeyRune {
				// 답안의 첫 글자가 키로 읽혔다 — 열이 새고 있다.
				rest = string(k.Rune)
				break
			}
			if k.Key == KeyUp || k.Key == KeyEnter {
				break
			}
		}
		line, _, _ := ReadLine(r)
		if rest+line != "답안" {
			t.Errorf("%q → 답안이 %q 가 됐다", strings.ReplaceAll(in, "|", ""), rest+line)
		}
	}
}

func TestKeyReaderReadsArrows(t *testing.T) {
	got, rest := keysFrom(t, "\x1b[A\x1b[B\x1bOC\x1b[D", 4)
	want := []Key{KeyUp, KeyDown, KeyRight, KeyLeft}
	if len(got) != len(want) {
		t.Fatalf("키 %d개, 기대 %d개: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Key != w {
			t.Errorf("%d번째 = %v, 기대 %v", i, got[i].Key, w)
		}
	}
	if rest != "" {
		t.Errorf("남은 것 %q", rest)
	}
}

func TestKeyReaderKeepsTypedTextAfterBareEscape(t *testing.T) {
	// ESC 뒤에 열이 아닌 글자가 오면 그 글자는 사용자가 친 것이다.
	got, rest := keysFrom(t, "\x1babc\r", 1)
	if len(got) != 1 || got[0].Key != KeyEscape {
		t.Fatalf("ESC를 못 읽었다: %+v", got)
	}
	if rest != "abc" {
		t.Errorf("친 글자가 사라졌다: %q", rest)
	}
}
