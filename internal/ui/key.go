package ui

import (
	"bufio"
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
// 한 줄씩 읽는 쪽과 **같은 버퍼**를 본다. 이것이 중요하다. 예전에는 파일에서
// 직접 한 덩어리를 읽어 남은 바이트를 자기 안에 들고 있었는데, 다음 화면이
// 한 줄 읽기로 넘어가면 그 바이트를 아무도 못 봤다. 초기화면에서 Enter를
// 누르고 곧바로 답을 쳐 넣으면 — 사람이 빠르게 치면 한 덩어리로 온다 —
// 답안이 통째로 사라졌다.
type KeyReader struct {
	in *bufio.Reader
	f  *os.File

	// state는 이스케이프 열을 읽던 중인지다.
	//
	// 터미널이 ESC 와 「[3~」를 나눠 건네주는 일이 있다. 남은 바이트를 그냥
	// 두면 다음 줄 읽기가 그것을 답안의 첫 글자로 읽는다 — 「[3~」로 시작하는
	// 답안이 저장되고 첨삭 요청으로 나가 값을 치른다. 다음 읽기에서 열
	// 해석을 이어가야 하고, 그러려면 어디까지 읽었는지 기억해야 한다.
	state seqState
}

type seqState int

const (
	seqNone     seqState = iota
	seqAfterEsc          // ESC만 받았다. 다음 바이트가 '[' 나 'O' 면 열이다
	seqInside            // 열 안이다. 마지막 글자가 올 때까지 삼킨다
)

// NewKeyReader는 in에서 키를 읽는 읽개를 만든다.
// f는 raw mode를 걸 대상이고, 읽기는 언제나 in으로 한다.
func NewKeyReader(in *bufio.Reader, f *os.File) *KeyReader {
	return &KeyReader{in: in, f: f}
}

// Read는 키 하나를 읽는다. 읽는 동안에만 터미널을 raw mode로 둔다.
//
// 키마다 raw mode를 껐다 켜는 것은 낭비지만, 그 대신 앱이 어디서 멈추든
// 터미널이 raw인 채로 남을 틈이 거의 없다. 포메라는 예고 없이 꺼지는
// 기계이고, 에코가 꺼진 셸을 기기 앞에서 되살리는 것은 성가신 일이다.
func (r *KeyReader) Read() (KeyPress, error) {
	fd := int(r.f.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return KeyPress{}, err
	}
	defer term.Restore(fd, state)
	return r.readNoRaw()
}

// readNoRaw는 터미널 모드를 건드리지 않고 키 하나를 읽는다.
// 상태 기계를 터미널 없이 시험할 수 있게 갈라 두었다.
func (r *KeyReader) readNoRaw() (KeyPress, error) {
	for {
		b, err := r.in.ReadByte()
		if err != nil {
			return KeyPress{Key: KeyQuit}, nil
		}

		switch r.state {
		case seqInside:
			// 열의 나머지를 삼킨다. CSI 열은 0x40~0x7E 의 글자로 끝난다.
			if b >= 0x40 && b <= 0x7e {
				r.state = seqNone
			}
			continue

		case seqAfterEsc:
			r.state = seqNone
			if b == '[' || b == 'O' {
				if k, ok := r.readSeq(b); ok {
					return k, nil
				}
				continue
			}
			// 열이 아니었다. 이 바이트를 보통 글자로 본다 — 아래로 흐른다.
		}

		switch b {
		case 0x03, 0x04: // Ctrl+C, Ctrl+D
			// raw mode에서는 Ctrl+C가 신호가 아니라 바이트로 온다. 신호였다면
			// defer가 돌지 못해 터미널이 raw인 채로 남는다 — 바이트로 받는
			// 편이 오히려 안전하다.
			return KeyPress{Key: KeyQuit}, nil
		case '\r', '\n':
			return KeyPress{Key: KeyEnter}, nil
		case 0x1b:
			if r.in.Buffered() == 0 {
				// 뒤가 아직 안 왔다. ESC로 넘기되 이어서 볼 수 있게 표시한다.
				// 여기서 더 읽으러 가면 ESC만 누른 사람을 다음 키를 누를
				// 때까지 기다리게 만든다.
				r.state = seqAfterEsc
				return KeyPress{Key: KeyEscape}, nil
			}
			c, err := r.in.ReadByte()
			if err != nil {
				return KeyPress{Key: KeyEscape}, nil
			}
			if c != '[' && c != 'O' {
				if err := r.in.UnreadByte(); err != nil {
					return KeyPress{Key: KeyEscape}, nil
				}
				return KeyPress{Key: KeyEscape}, nil
			}
			if k, ok := r.readSeq(c); ok {
				return k, nil
			}
			continue
		}

		if b < utf8.RuneSelf {
			return KeyPress{Key: KeyRune, Rune: rune(b)}, nil
		}
		return KeyPress{Key: KeyRune, Rune: r.rune(b)}, nil
	}
}

// readSeq는 「ESC [」 또는 「ESC O」 다음을 읽는다.
//
// 화살표면 그 키를 돌려준다. 그 밖의 열(Delete·Home·기능키)은 통째로
// 삼키고 ok=false를 돌려준다 — 부르는 쪽은 다음 키를 계속 읽는다.
// 열이 아직 안 끝났으면 다음 읽기에서 이어가도록 표시한다.
func (r *KeyReader) readSeq(_ byte) (KeyPress, bool) {
	if r.in.Buffered() == 0 {
		r.state = seqInside
		return KeyPress{}, false
	}
	b, err := r.in.ReadByte()
	if err != nil {
		return KeyPress{}, false
	}
	// ESC [ A 와 ESC O A 를 둘 다 받는다. 커서 키는 터미널이 어느 모드에
	// 있느냐에 따라 두 꼴로 온다.
	switch b {
	case 'A':
		return KeyPress{Key: KeyUp}, true
	case 'B':
		return KeyPress{Key: KeyDown}, true
	case 'C':
		return KeyPress{Key: KeyRight}, true
	case 'D':
		return KeyPress{Key: KeyLeft}, true
	}
	// 모르는 열이다. 마지막 글자까지 삼킨다.
	if b >= 0x40 && b <= 0x7e {
		return KeyPress{}, false
	}
	r.state = seqInside
	return KeyPress{}, false
}

// rune은 첫 바이트를 받은 뒤 나머지를 마저 읽어 글자 하나를 만든다.
func (r *KeyReader) rune(first byte) rune {
	n := 1
	switch {
	case first&0xE0 == 0xC0:
		n = 2
	case first&0xF0 == 0xE0:
		n = 3
	case first&0xF8 == 0xF0:
		n = 4
	}
	buf := []byte{first}
	for len(buf) < n && r.in.Buffered() > 0 {
		b, err := r.in.ReadByte()
		if err != nil {
			break
		}
		buf = append(buf, b)
	}
	c, _ := utf8.DecodeRune(buf)
	return c
}
