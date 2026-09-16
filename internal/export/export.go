// Package export는 첨삭을 순정 포메라 모드에서 읽을 텍스트로 내보낸다.
package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
	tsync "github.com/noel88/tsuzuri/internal/sync"
)

// Write는 받은 첨삭을 review/ 아래 .txt로 쓴다.
//
// SD카드의 vfat 공유 영역이므로 Debian을 띄우지 않고 순정 포메라 모드에서
// 그대로 열어 읽을 수 있다. 그래서 CRLF로 쓴다.
//
// 노트 경로와 건수를 돌려준다. 첨삭이 없으면 파일을 만들지 않는다.
func Write(dataDir string, now time.Time) (string, int, error) {
	feedback, err := store.ReadAll[tsync.Feedback](filepath.Join(dataDir, "feedback.jsonl"))
	if err != nil {
		return "", 0, err
	}
	if len(feedback) == 0 {
		return "", 0, nil
	}

	attempts, err := store.ReadAll[store.Attempt](filepath.Join(dataDir, "attempts.jsonl"))
	if err != nil {
		return "", 0, err
	}
	byID := make(map[string]store.Attempt, len(attempts))
	for _, a := range attempts {
		byID[a.ID] = a
	}

	problems, _, err := pack.ByID(filepath.Join(dataDir, "packs"))
	if err != nil {
		return "", 0, err
	}

	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\r\n") }

	line("Tsuzuri 복습 노트")
	line(now.Format("2006-01-02 15:04"))
	line(strings.Repeat("=", 40))
	line("")

	n := 0
	for _, f := range feedback {
		a, ok := byID[f.AttemptID]
		if !ok {
			continue
		}
		p, ok := problems[a.PackID]
		if !ok {
			// 팩이 사라졌다. 제시문도 참조도 없이 내보내면 읽을 수 없다.
			continue
		}

		line(fmt.Sprintf("[%s] %s %s", p.ID, p.Level, p.Topic))
		line("제시문: " + p.Prompt)
		line("내 답 : " + a.Answer)
		if f.Corrected != "" {
			line("첨삭  : " + f.Corrected)
		}
		line("참조  : " + p.Reference)
		for _, note := range f.Notes {
			mark := "  * "
			if note.Level == tsync.LevelNuance {
				mark = "  - "
			}
			line(mark + note.Span + " : " + note.Why)
		}
		if f.Overall != "" {
			line("  " + f.Overall)
		}
		line("")
		n++
	}
	if n == 0 {
		return "", 0, nil
	}

	outDir := filepath.Join(dataDir, "review")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", 0, err
	}
	path := filepath.Join(outDir, "review-"+now.Format("20060102-150405")+".txt")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", 0, err
	}
	return path, n, nil
}
