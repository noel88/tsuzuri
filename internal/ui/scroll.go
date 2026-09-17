package ui

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// fallbackHeight는 화면 높이를 알아낼 수 없을 때 쓰는 값이다.
// 포메라 fbterm은 36줄이다.
const fallbackHeight = 24

// TermHeight는 화면이 몇 줄인지 돌려준다.
func TermHeight() int {
	if _, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && saneHeight(h) {
		return h
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("LINES"))); err == nil && saneHeight(n) {
		return n
	}
	return fallbackHeight
}

func saneHeight(n int) bool { return n >= 10 && n <= 200 }

// barLines는 화면 맨 아래 명령줄이 차지하는 줄 수다 (CommandBarWith).
const barLines = 3

// Scroll은 화면 하나를 잘라 보여줄 수 있게 한다.
//
// 문단 하나를 통째로 번역하는 문항이 생기면서 제시문·답안·참조가 한 화면에
// 들어가지 않는다. 명령줄은 늘 맨 아래에 붙여 둔다 — 본문이 길다고 무엇을
// 누를 수 있는지가 화면 밖으로 밀려나면 안 된다.
//
// max는 더 내려갈 수 있는 마지막 자리다. 부르는 쪽이 이것으로 화살표를
// 막는다. 본문이 화면에 다 들어가면 0이다.
func Scroll(page string, rows, offset int) (out string, max int) {
	lines := strings.Split(page, "\n")
	if len(lines) <= barLines || rows < barLines+4 {
		return page, 0
	}
	body, bar := lines[:len(lines)-barLines], lines[len(lines)-barLines:]

	// 본문에 쓸 수 있는 줄 수. 한 줄은 「더 있습니다」 표시에 남긴다.
	room := rows - barLines - 1
	if len(body) <= room {
		return page, 0
	}

	max = len(body) - room
	if offset < 0 {
		offset = 0
	}
	if offset > max {
		offset = max
	}

	shown := append([]string(nil), body[offset:offset+room]...)
	shown = append(shown, scrollHint(offset, max))
	return strings.Join(append(shown, bar...), "\n"), max
}

// scrollHint는 위아래로 더 있다는 것을 알린다.
//
// 화살표 글리프를 쓰지 않는다. 「↑」는 폭이 애매한 문자라 터미널마다 한 칸
// 또는 두 칸으로 그려지고, 그것 때문에 실기에서 줄이 접힌 적이 있다.
func scrollHint(offset, max int) string {
	switch {
	case offset == 0:
		return "   (아래로 더 있습니다 — 위아래 화살표)"
	case offset >= max:
		return "   (여기가 끝입니다 — 위아래 화살표)"
	default:
		return "   (위아래로 더 있습니다 — 위아래 화살표)"
	}
}
