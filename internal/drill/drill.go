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
	"github.com/noel88/tsuzuri/internal/progress"
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
	Online   bool // 마지막 온라인 작업이 성공했는지. 화면 상태 표시에 쓴다.
	Now      func() time.Time

	// Start는 몇 번째 문제부터 낼지다 (0부터). 이어하기에 쓴다.
	Start int
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
	marksPath := filepath.Join(s.DataDir, "marks.jsonl")

	queued, err := store.ReadAll[store.QueueItem](queuePath)
	if err != nil {
		return OutcomeDone, err
	}
	queueLen := len(queued)

	start := s.Start
	if start < 0 || start >= len(s.Problems) {
		start = 0
	}
	for i := start; i < len(s.Problems); {
		p := s.Problems[i]
		st := ui.Status{Index: i + 1, Total: len(s.Problems), QueueLen: queueLen, Online: s.Online}

		ui.Clear(s.Out)
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

		// 답안을 저장한 직후에 큐에 넣는다.
		//
		// 결과 화면의 입력을 기다렸다가 넣으면, 그 화면에서 덮개를 닫거나
		// Ctrl+C를 누른 답안이 저장은 됐는데 첨삭 대기열에는 없는 상태가
		// 된다. 그런 답안은 영영 첨삭받지 못한다.
		if err := store.Append(queuePath, store.QueueItem{
			AttemptID: attempt.ID,
			At:        at,
		}); err != nil {
			return OutcomeDone, err
		}
		queueLen++
		st.QueueLen = queueLen

		ui.Clear(s.Out)
		fmt.Fprint(s.Out, ui.RenderResult(p, answer, a, st, s.TermW))

		cmdLine, cmdEOF, err := ui.ReadLine(s.In)
		if err != nil {
			return OutcomeDone, err
		}
		cmd := ui.ParseCommand(cmdLine)

		// B는 복습 표시다. 첨삭이 조용히 넘어간 것이라도 스스로 다시 보고
		// 싶을 때가 있다 — 답은 맞았지만 자신이 없었던 문장 같은 것.
		if cmd == ui.CmdMark {
			if err := store.Append(marksPath, progress.Mark{
				ProblemID: p.ID,
				At:        at,
				Kind:      progress.KindFlag,
			}); err != nil {
				return OutcomeDone, err
			}
		}

		// F는 우선 처리 표시다. 큐는 append-only이므로 같은 답안에 대해
		// 우선 표시를 한 줄 더 남긴다. sync가 답안 단위로 합친다.
		if cmd == ui.CmdPriority {
			if err := store.Append(queuePath, store.QueueItem{
				AttemptID: attempt.ID,
				Priority:  true,
				At:        at,
			}); err != nil {
				return OutcomeDone, err
			}
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
