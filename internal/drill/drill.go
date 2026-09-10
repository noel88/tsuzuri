package drill

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
	"github.com/noel88/tsuzuri/internal/ui"
)

// Outcome은 드릴 세션이 어떻게 끝났는지다.
type Outcome int

const (
	OutcomeDone Outcome = iota // 문제를 다 풀었거나 입력이 끝났다
	OutcomeMenu                // 사용자가 메뉴로 돌아갔다
	OutcomeQuit                // 사용자가 종료했다
)

// Session은 드릴 한 회차다. 한 방향만 다룬다.
//
// 네트워크를 쓰지 않는다. 이 패키지는 HTTP 클라이언트를 알지 못한다.
type Session struct {
	Problems []pack.Problem
	Analyzer *analyze.Analyzer
	DataDir  string
	In       *bufio.Reader
	Out      io.Writer
	TermW    int
	Now      func() time.Time
}

// Run은 문제를 순서대로 출제하고 답안과 분석 결과를 저장한다.
func (s *Session) Run() (Outcome, error) {
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.TermW <= 0 {
		s.TermW = ui.TermWidth()
	}

	attemptsPath := filepath.Join(s.DataDir, "attempts.jsonl")
	queuePath := filepath.Join(s.DataDir, "queue.jsonl")

	queued, err := store.ReadAll[store.QueueItem](queuePath)
	if err != nil {
		return OutcomeDone, err
	}
	queueLen := len(queued)

	for i := 0; i < len(s.Problems); {
		p := s.Problems[i]
		st := ui.Status{Index: i + 1, Total: len(s.Problems), QueueLen: queueLen}

		fmt.Fprint(s.Out, ui.RenderProblem(p, st, s.TermW))

		answer, eof, err := s.readAnswer()
		if err != nil {
			return OutcomeDone, err
		}
		trimmed := strings.TrimSpace(answer)

		// 답안 자리에서도 M/X로 빠져나갈 수 있다.
		if trimmed != "" {
			switch ui.ParseCommand(trimmed) {
			case ui.CmdMenu:
				return OutcomeMenu, nil
			case ui.CmdQuit:
				return OutcomeQuit, nil
			}
		}
		if trimmed == "" {
			if eof {
				return OutcomeDone, nil
			}
			i++
			continue
		}

		a := s.Analyzer.Analyze(p, answer)
		at := s.Now()
		attempt := store.Attempt{
			ID:       fmt.Sprintf("a%d-%s", at.UnixNano(), p.ID),
			PackID:   p.ID,
			At:       at,
			Answer:   answer,
			Analysis: a,
		}
		if err := store.Append(attemptsPath, attempt); err != nil {
			return OutcomeDone, err
		}
		queueLen++
		st.QueueLen = queueLen

		fmt.Fprint(s.Out, ui.RenderResult(p, answer, a, st, s.TermW))

		cmdLine, cmdEOF, err := ui.ReadLine(s.In)
		if err != nil {
			return OutcomeDone, err
		}
		cmd := ui.ParseCommand(cmdLine)

		// 모든 답안을 큐에 적재한다. F는 우선 처리 표시일 뿐이다.
		if err := store.Append(queuePath, store.QueueItem{
			AttemptID: attempt.ID,
			Priority:  cmd == ui.CmdPriority,
			At:        at,
		}); err != nil {
			return OutcomeDone, err
		}

		switch cmd {
		case ui.CmdQuit:
			return OutcomeQuit, nil
		case ui.CmdMenu:
			return OutcomeMenu, nil
		case ui.CmdRetry:
			// 같은 문제를 다시 낸다. 이전 attempt는 지우지 않는다.
		default:
			i++
		}
		if cmdEOF {
			return OutcomeDone, nil
		}
	}
	return OutcomeDone, nil
}

// readAnswer는 답안 한 줄을 읽되, ":e"면 에디터를 연다.
func (s *Session) readAnswer() (string, bool, error) {
	line, eof, err := ui.ReadLine(s.In)
	if err != nil {
		return "", eof, err
	}
	if ui.IsEditorRequest(line) {
		text, err := ui.ReadFromEditor("", "")
		return text, eof, err
	}
	return line, eof, nil
}
