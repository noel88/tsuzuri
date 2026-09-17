package drill

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
)

// darwin pty
const (
	tiocptygrant = 0x20007454
	tiocptyunlk  = 0x20007452
	tiocptygname = 0x40807453
)

func openPty(t *testing.T) (master, slave *os.File) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("ptmx: %v", err)
	}
	fd := m.Fd()
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocptygrant, 0); e != 0 {
		t.Skipf("grant: %v", e)
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocptyunlk, 0); e != 0 {
		t.Skipf("unlk: %v", e)
	}
	var buf [128]byte
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocptygname, uintptr(unsafe.Pointer(&buf[0]))); e != 0 {
		t.Skipf("gname: %v", e)
	}
	n := bytes.IndexByte(buf[:], 0)
	s, err := os.OpenFile(string(buf[:n]), os.O_RDWR, 0)
	if err != nil {
		t.Skipf("slave: %v", err)
	}
	return m, s
}

func ptySession(t *testing.T, dir string, slave *os.File) *Session {
	t.Helper()
	az, err := analyze.New(pack.KoToJa)
	if err != nil {
		t.Fatalf("analyze.New: %v", err)
	}
	var out bytes.Buffer
	return &Session{
		Problems: problems(),
		Analyzer: az,
		DataDir:  dir,
		In:       bufio.NewReader(slave),
		Out:      &out,
		TermW:    92,
		Now:      fixedNow,
		Keys:     slave,
	}
}

// 답안 화면의 고르기 줄에서 B/R/F 를 누르면 어떻게 되는가.
func TestAnswerActionLetterKeys(t *testing.T) {
	for _, key := range []string{"b", "r", "f"} {
		t.Run(key, func(t *testing.T) {
			master, slave := openPty(t)
			defer master.Close()
			defer slave.Close()

			// os.Stdin 이 tty 여야 ui.Interactive() 가 true 다.
			old := os.Stdin
			os.Stdin = slave
			defer func() { os.Stdin = old }()

			dir := t.TempDir()
			s := ptySession(t, dir, slave)

			done := make(chan Outcome, 1)
			go func() {
				o, err := s.Run()
				if err != nil {
					t.Errorf("Run: %v", err)
				}
				done <- o
			}()

			// 빈 줄 ⏎ → 고르기 줄
			master.WriteString("\n")
			time.Sleep(300 * time.Millisecond)
			// 글자 키
			master.WriteString(key)
			time.Sleep(300 * time.Millisecond)
			// 다음 문제 화면이 떴으면 여기서 빈 줄 → 고르기 → 종료
			master.WriteString("\n")
			time.Sleep(300 * time.Millisecond)
			master.WriteString("x")

			select {
			case o := <-done:
				t.Logf("key=%q outcome=%v", key, o)
			case <-time.After(10 * time.Second):
				t.Fatalf("key=%q: 끝나지 않았다", key)
			}

			attempts, _ := store.ReadAll[store.Attempt](filepath.Join(dir, "attempts.jsonl"))
			out := s.Out.(*bytes.Buffer).String()
			t.Logf("attempts=%d, p002 화면에 나왔나=%v", len(attempts),
				bytes.Contains([]byte(out), []byte("비가 온다")))
		})
	}
}
