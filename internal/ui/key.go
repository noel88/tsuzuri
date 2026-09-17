package ui

import (
	"os"
	"unicode/utf8"

	"golang.org/x/term"
)

// Key는 키 하나다.
type Key int

const (
	KeyRune   Key = iota // 보통 글자. KeyPress.Rune에 담긴다
	KeyUp                // ↑
	KeyDown              // ↓
	KeyLeft              // ←
	KeyRight             // →
	KeyEnter             // ⏎
	KeyEscape            // ESC
	KeyQuit              // Ctrl+C, Ctrl+D
)

// KeyPress는 읽어들인 키다.
type KeyPress struct {
	Key  Key
	Rune rune
}

// arrowsOff는 화살표 조작을 끄는 환경변수다.
//
// 이 앱은 터미널을 raw mode로 바꿔 키를 하나씩 읽는 일을 오래 피해 왔다
// (스펙 §6.1). 입력기가 조합 중인 글자를 어떻게 다룰지 보장이 없어서다.
// 고르기만 하는 화면에는 입력기가 필요 없으므로 거기서만 쓰지만, 기기에서
// 어긋나는 것이 보이면 이 스위치로 통째로 되돌릴 수 있어야 한다.
const arrowsOff = "TSUZURI_NO_ARROWS"

// Interactive는 키를 하나씩 읽을 수 있는 상태인지 본다.
//
// 입력이 파이프나 파일이면 raw mode가 성립하지 않는다. 그때는 부르는 쪽이
// 예전처럼 한 줄을 읽어야 한다 — 캡처 도구와 스크립트가 그 경로로 돈다.
func Interactive() bool {
	if os.Getenv(arrowsOff) != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// KeyReader는 키를 하나씩 읽는다.
//
// 아직 다 쓰지 않은 바이트를 들고 있어야 한다. 화살표는 ESC [ A 처럼 세
// 바이트로 오고 터미널은 그것을 한 덩어리로 건네주는데, 키를 연달아 누르면
// 한 번의 읽기에 여러 키가 함께 온다. 읽은 만큼 버리면 두 번째부터 씹힌다 —
// 화살표를 꾹 누르면 한 칸만 움직였다.
type KeyReader struct {
	f    *os.File
	rest []byte
}

// NewKeyReader는 f에서 키를 읽는 읽개를 만든다.
func NewKeyReader(f *os.File) *KeyReader { return &KeyReader{f: f} }

// Read는 키 하나를 읽는다. 읽는 동안에만 터미널을 raw mode로 둔다.
//
// 키마다 raw mode를 껐다 켜는 것은 낭비지만, 그 대신 앱이 어디서 멈추든
// 터미널이 raw인 채로 남을 틈이 거의 없다. 포메라는 예고 없이 꺼지는
// 기계이고, 에코가 꺼진 셸을 기기 앞에서 되살리는 것은 성가신 일이다.
func (r *KeyReader) Read() (KeyPress, error) {
	if len(r.rest) == 0 {
		fd := int(r.f.Fd())
		state, err := term.MakeRaw(fd)
		if err != nil {
			return KeyPress{}, err
		}
		buf := make([]byte, 32)
		n, err := r.f.Read(buf)
		term.Restore(fd, state)
		if err != nil || n == 0 {
			return KeyPress{Key: KeyQuit}, nil
		}
		r.rest = buf[:n]
	}

	k, used := parseKey(r.rest, r.f)
	r.rest = r.rest[used:]
	return k, nil
}

// parseKey는 앞에서부터 키 하나를 떼어 내고, 쓴 바이트 수를 함께 돌려준다.
func parseKey(b []byte, f *os.File) (KeyPress, int) {
	switch b[0] {
	case 0x03, 0x04: // Ctrl+C, Ctrl+D
		// raw mode에서는 Ctrl+C가 신호가 아니라 바이트로 온다. 신호였다면
		// defer가 돌지 못해 터미널이 raw인 채로 남는다 — 바이트로 받는
		// 편이 오히려 안전하다.
		return KeyPress{Key: KeyQuit}, 1
	case '\r', '\n':
		return KeyPress{Key: KeyEnter}, 1
	case 0x1b:
		return parseEscape(b)
	}

	if b[0] < utf8.RuneSelf {
		return KeyPress{Key: KeyRune, Rune: rune(b[0])}, 1
	}
	r, size := utf8.DecodeRune(b)
	if r == utf8.RuneError && size <= 1 {
		// 글자가 덩어리에 걸쳐 잘렸다. 나머지를 마저 읽는다.
		return KeyPress{Key: KeyRune, Rune: readRune(f, b)}, len(b)
	}
	return KeyPress{Key: KeyRune, Rune: r}, size
}

// parseEscape는 ESC로 시작하는 덩어리를 본다. ESC뿐이면 ESC 자체다.
func parseEscape(b []byte) (KeyPress, int) {
	if len(b) < 3 {
		return KeyPress{Key: KeyEscape}, 1
	}
	// ESC [ A 와 ESC O A 를 둘 다 받는다. 커서 키는 터미널이 어느 모드에
	// 있느냐에 따라 두 꼴로 온다.
	if b[1] != '[' && b[1] != 'O' {
		return KeyPress{Key: KeyEscape}, 1
	}
	switch b[2] {
	case 'A':
		return KeyPress{Key: KeyUp}, 3
	case 'B':
		return KeyPress{Key: KeyDown}, 3
	case 'C':
		return KeyPress{Key: KeyRight}, 3
	case 'D':
		return KeyPress{Key: KeyLeft}, 3
	}
	// 모르는 열이다. ESC 하나만 쓰고 나머지는 다음 차례에 본다 —
	// 통째로 버리면 뒤에 붙어 온 진짜 키까지 잃는다.
	return KeyPress{Key: KeyEscape}, 1
}

// readRune은 잘린 글자의 나머지를 마저 읽어 글자 하나를 만든다.
func readRune(f *os.File, head []byte) rune {
	n := 1
	switch {
	case head[0]&0xE0 == 0xC0:
		n = 2
	case head[0]&0xF0 == 0xE0:
		n = 3
	case head[0]&0xF8 == 0xF0:
		n = 4
	}
	buf := append([]byte(nil), head...)
	for len(buf) < n {
		var b [1]byte
		if _, err := f.Read(b[:]); err != nil {
			break
		}
		buf = append(buf, b[0])
	}
	r, _ := utf8.DecodeRune(buf)
	return r
}
