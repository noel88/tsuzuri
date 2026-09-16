package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"
)

// Attempt는 사용자가 제출한 답안 하나다.
//
// Analysis가 any인 것은 의도적이다. 저장소는 분석 결과의 형태를 알 필요가 없고,
// 알면 analyze 패키지에 결합된다.
type Attempt struct {
	ID       string    `json:"id"`
	PackID   string    `json:"pack_id"`
	At       time.Time `json:"at"`
	Answer   string    `json:"answer"`
	Analysis any       `json:"analysis,omitempty"`
}

// QueueItem은 첨삭 대기 항목이다.
// 모든 답안이 큐에 들어가며, Priority는 우선 처리 표시일 뿐이다.
type QueueItem struct {
	AttemptID string    `json:"attempt_id"`
	Priority  bool      `json:"priority"`
	At        time.Time `json:"at"`
}

// Append는 v를 JSON 한 줄로 파일 끝에 덧붙이고 디스크에 flush한다.
// 기존 내용은 절대 수정하지 않는다.
func Append(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return appendRaw(path, string(b))
}

func appendRaw(path, line string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 이전 쓰기가 전원 차단으로 잘려 있으면 그 조각을 먼저 잘라낸다.
	//
	// 그냥 이어 붙이면 다음 기록이 조각에 들러붙어 한 줄이 되고, 그 줄은
	// 더 이상 마지막 줄이 아니게 된다. ReadAll은 마지막 줄의 손상만
	// 허용하므로 그때부터 파일 전체를 읽지 못하고 앱이 시작조차 못 한다.
	end, err := truncateTornTail(f)
	if err != nil {
		return err
	}
	if _, err := f.Seek(end, io.SeekStart); err != nil {
		return err
	}

	if _, err := f.WriteString(line + "\n"); err != nil {
		return err
	}
	// 포메라는 예고 없이 꺼진다. 매 쓰기마다 디스크까지 내린다.
	return f.Sync()
}

// truncateTornTail은 마지막 개행 뒤에 남은 조각을 잘라내고,
// 이어 쓸 위치를 돌려준다. 파일이 온전하면 크기를 그대로 돌려준다.
func truncateTornTail(f *os.File) (int64, error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := fi.Size()
	if size == 0 {
		return 0, nil
	}

	// 끝에서부터 개행을 찾는다. 한 줄이 아주 길 수 있으므로 블록 단위로 본다.
	const block = 64 * 1024
	buf := make([]byte, block)
	for pos := size; pos > 0; {
		n := int64(block)
		if pos < n {
			n = pos
		}
		pos -= n
		if _, err := f.ReadAt(buf[:n], pos); err != nil {
			return 0, err
		}
		if i := bytes.LastIndexByte(buf[:n], '\n'); i >= 0 {
			end := pos + int64(i) + 1
			if end == size {
				return size, nil // 온전하다
			}
			return end, f.Truncate(end)
		}
	}
	// 개행이 하나도 없다 — 파일 전체가 조각이다.
	return 0, f.Truncate(0)
}

// ReadAll은 JSON Lines 파일 전체를 읽는다.
//
// 파일이 없으면 빈 슬라이스를 돌려준다.
// 마지막 줄이 손상됐으면(전원 차단) 그 줄만 버린다 — append-only이므로
// 손상될 수 있는 것은 마지막 줄뿐이다. 중간 줄이 깨졌다면 그것은
// 전원 차단이 아니라 다른 문제이므로 오류로 보고한다.
func ReadAll[T any](path string) ([]T, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}

	out := make([]T, 0, len(lines))
	for i, text := range lines {
		var v T
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			if i == len(lines)-1 {
				// 마지막 줄만 잘림을 허용한다.
				break
			}
			return nil, fmt.Errorf("%s %d번째 줄이 손상됐다: %w", path, i+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// readLines는 빈 줄을 제외한 모든 줄을 순서대로 돌려준다.
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if text := strings.TrimSpace(sc.Text()); text != "" {
			lines = append(lines, text)
		}
	}
	return lines, sc.Err()
}
