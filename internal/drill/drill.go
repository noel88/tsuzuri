package drill

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

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

	// Rows는 화면이 몇 줄인지다. 0이면 터미널에 물어본다.
	Rows int

	// Keys는 키를 하나씩 읽을 입력이다. 비면 예전처럼 한 줄씩 읽는다.
	//
	// In과 따로 두는 것은 raw mode가 파일 서술자에 거는 것이라서다.
	// 테스트와 캡처 도구는 여기를 비워 두고 In으로만 넣는다.
	Keys *os.File

	keys *ui.KeyReader

	// warn은 다음 출제 화면 위에 한 번 띄울 알림이다.
	warn string
}

// Run은 문제를 순서대로 출제하고 답안과 분석 결과를 저장한다.
func (s *Session) Run() (Outcome, error) {
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.TermW <= 0 {
		s.TermW = ui.TermWidth()
	}
	if s.Keys != nil && ui.Interactive() {
		s.keys = ui.NewKeyReader(s.In, s.Keys)
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

		answer, cmd, eof, err := s.askAnswer(p, st)
		if err != nil {
			return OutcomeDone, err
		}
		switch cmd {
		case ui.CmdMenu:
			return OutcomeMenu, nil
		case ui.CmdQuit:
			return OutcomeQuit, nil
		case ui.CmdNext:
			if eof {
				return OutcomeDone, nil
			}
			i++
			continue
		}
		// 에디터를 열었다가 빈 채로 닫는 일이 있다. 빈 답안을 저장하면
		// 첨삭 대기열에 들어가 값을 치르고, 진도에도 푼 것으로 잡힌다.
		if strings.TrimSpace(answer) == "" {
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

		cmd, cmdEOF, err := s.readResultCommand(p, answer, a, st)
		if err != nil {
			return OutcomeDone, err
		}

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

// readResultCommand는 결과 화면을 그리고 다음 동작을 고르게 한다.
//
// 키를 하나씩 읽을 수 있으면 좌우 화살표로 고르고, 아니면 예전처럼 한 줄을
// 읽는다. 파이프로 입력을 넣는 자리(캡처 도구, 스크립트)가 그 경로로 돈다.
func (s *Session) readResultCommand(p pack.Problem, answer string, a analyze.Analysis, st ui.Status) (ui.Command, bool, error) {
	offset, max := 0, 0
	draw := func(sel int) {
		page, m := ui.Scroll(ui.RenderResult(p, answer, a, st, s.TermW, sel), s.rows(), offset)
		max = m
		ui.Clear(s.Out)
		fmt.Fprint(s.Out, page)
	}

	if s.keys == nil {
		draw(0)
		line, eof, err := ui.ReadLine(s.In)
		if err != nil {
			return ui.CmdNext, eof, err
		}
		return ui.ParseCommand(line), eof, nil
	}

	sel := 0
	for {
		draw(sel)
		k, err := s.keys.Read()
		if err != nil {
			// raw mode가 안 되는 터미널이다. 한 줄 읽기로 물러난다.
			line, eof, err := ui.ReadLine(s.In)
			if err != nil {
				return ui.CmdNext, eof, err
			}
			return ui.ParseCommand(line), eof, nil
		}
		switch k.Key {
		case ui.KeyQuit:
			return ui.CmdQuit, true, nil
		case ui.KeyEnter:
			return ui.ActionAt(ui.ResultActions, sel), false, nil
		case ui.KeyLeft, ui.KeyRight:
			sel = ui.MoveAction(sel, k.Key, len(ui.ResultActions))
		case ui.KeyUp:
			// 문단 하나를 번역한 답안은 한 화면에 안 들어간다.
			if offset > 0 {
				offset--
			}
		case ui.KeyDown:
			if offset < max {
				offset++
			}
		case ui.KeyRune:
			// 글자 키도 그대로 받는다. 입력기가 켜져 있으면 글자가
			// 조합으로 먹히는데, 그때는 화살표가 대신한다.
			if cmd, ok := ui.ActionByKey(ui.ResultActions, k.Rune); ok {
				return cmd, false, nil
			}
		}
		// ESC는 아무것도 하지 않는다. 예전에는 메뉴로 나갔는데, 화살표의
		// ESC와 구별하려면 뒤에 무엇이 붙어 오는지를 봐야 한다. 터미널이
		// 세 바이트를 나눠 건네주는 순간에 방향키 한 번으로 드릴이
		// 끝나 버린다. 메뉴로는 M이나 고르기 줄로 나간다.
	}
}

// rows는 화면 높이다.
func (s *Session) rows() int {
	if s.Rows > 0 {
		return s.Rows
	}
	return ui.TermHeight()
}

// askAnswer는 답안을 받는다. 답 대신 다른 일을 고르면 그 명령을 돌려준다.
//
// 답을 치는 동안에는 키를 하나씩 받지 않는다. 그 화면은 터미널의 보통
// 입력 경로를 그대로 써야 입력기가 동작하고 친 글자가 화면에 보인다.
// 빈 줄에서 Enter를 누른 순간에만 — 아무것도 치고 있지 않을 때만 —
// 고르기 줄을 띄운다.
func (s *Session) askAnswer(p pack.Problem, st ui.Status) (string, ui.Command, bool, error) {
	for {
		st.Warn = s.warn
		s.warn = ""
		page, _ := ui.Scroll(ui.RenderProblem(p, st, s.TermW, -1), s.rows(), 0)
		ui.Clear(s.Out)
		fmt.Fprint(s.Out, page)

		line, eof, err := ui.ReadLine(s.In)
		if err != nil {
			return "", ui.CmdStay, eof, err
		}
		if p.IsLong() && strings.TrimSpace(line) == "" && !eof {
			// 문단 하나를 한 줄 입력으로 쓸 수는 없다. 긴 문항은 곧바로
			// 편집기를 연다 — 여기서 고르기 줄을 띄우면 매번 「긴 답」을
			// 골라야 하고, 그것 말고 고를 것도 없다.
			text, err := ui.ReadFromEditor("", "")
			if err != nil {
				return "", ui.CmdStay, false, err
			}
			if strings.TrimSpace(text) != "" {
				return text, ui.CmdStay, false, nil
			}
			// 편집기를 저장 없이 닫았다. 그대로 넘기면 문항 하나가 기록도
			// 없이 사라진다 — 잘못 연 것일 수도 있으니 무엇을 할지 묻는다.
			switch cmd, err := s.editorCancelled(p, st); {
			case err != nil:
				return "", ui.CmdStay, false, err
			case cmd == ui.CmdEdit:
				// 「긴 답」을 고르면 편집기를 다시 연다. 잘못 닫은 뒤
				// 고르는 가장 자연스러운 선택지인데, 예전에는 그것이
				// 문항 버리기로 동작했다.
				continue
			case cmd == ui.CmdStay:
				// 다시 답 치는 화면으로.
			default:
				return "", cmd, false, nil
			}
			continue
		}
		trimmed := strings.TrimSpace(line)

		if ui.IsEditorRequest(line) {
			text, err := ui.ReadFromEditor("", "")
			if err != nil || strings.TrimSpace(text) != "" {
				return text, ui.CmdStay, eof, err
			}
			continue
		}
		// 입력기가 글자를 중간에서 자른 답안을 저장하지 않는다.
		//
		// 조합 중에 Enter가 들어가면 마지막 글자의 바이트 일부만 온다.
		// 설정 파일에서 먼저 겪었고, 실기에서 일본어 답안에도 났다 —
		// 「休」 뒤에 깨진 바이트가 붙은 답안이 저장되고 첨삭까지 나가서,
		// 모델이 「표기가 무너져 일본어로 성립하지 않는다」고 답했다.
		// 값을 치르고 받은 지적이 자기 오타에 대한 것이면 아무 쓸모가 없다.
		if line != "" && !utf8.ValidString(line) {
			// 입력을 더 받지 않는다. 여기서 한 줄을 읽으면 사용자가 이어서
			// 다시 친 답안이 그 자리에 먹힌다.
			s.warn = "입력한 글자가 깨졌습니다. 조합하던 중에 Enter를 누르면 마지막 글자가 " +
				"잘립니다 — 다 확정된 뒤에 다시 쳐 주세요."
			continue
		}
		// 팩 받기 프롬프트에서 쓰는 :q도 여기서 받는다. 받지 않으면
		// 그것이 답안으로 저장되어 첨삭 요청으로 나가고 값을 치른다.
		if ui.IsCancel(trimmed) {
			return "", ui.CmdMenu, eof, nil
		}
		// 답안 자리에서도 M/X로 빠져나갈 수 있다.
		if trimmed != "" {
			switch cmd := ui.ParseCommand(trimmed); cmd {
			case ui.CmdMenu, ui.CmdQuit:
				return "", cmd, eof, nil
			}
			return line, ui.CmdStay, eof, nil
		}

		if eof || s.keys == nil {
			// 예전 그대로 — 빈 줄은 건너뛰기다.
			return "", ui.CmdNext, eof, nil
		}

		switch cmd, err := s.chooseAnswerAction(p, st); {
		case err != nil:
			return "", ui.CmdStay, false, err
		case cmd == ui.CmdEdit:
			text, err := ui.ReadFromEditor("", "")
			if err != nil || strings.TrimSpace(text) != "" {
				return text, ui.CmdStay, false, err
			}
			// 저장 없이 닫았다. 다시 답 치는 화면으로.
		case cmd == ui.CmdStay:
			// 다시 답 치는 화면으로.
		default:
			return "", cmd, false, nil
		}
	}
}

// editorCancelled는 긴 문항에서 편집기를 저장 없이 닫았을 때 무엇을 할지
// 묻는다. 긴 문항은 고르기 줄을 거치지 않으므로 여기서 한 번 띄운다.
func (s *Session) editorCancelled(p pack.Problem, st ui.Status) (ui.Command, error) {
	if s.keys == nil {
		return ui.CmdNext, nil
	}
	return s.chooseAnswerAction(p, st)
}

// chooseAnswerAction은 답안 화면의 고르기 줄을 돌린다.
func (s *Session) chooseAnswerAction(p pack.Problem, st ui.Status) (ui.Command, error) {
	sel := 0
	for {
		ui.Clear(s.Out)
		fmt.Fprint(s.Out, ui.RenderProblem(p, st, s.TermW, sel))

		k, err := s.keys.Read()
		if err != nil {
			// raw mode가 안 되는 터미널이다. 결과 화면과 같게 물러난다.
			return ui.CmdStay, nil
		}
		switch k.Key {
		case ui.KeyQuit:
			return ui.CmdQuit, nil
		case ui.KeyEnter:
			return ui.ActionAt(ui.AnswerActions, sel), nil
		case ui.KeyLeft, ui.KeyRight:
			sel = ui.MoveAction(sel, k.Key, len(ui.AnswerActions))
		case ui.KeyEscape:
			return ui.CmdStay, nil
		case ui.KeyRune:
			// 글자를 치기 시작했으면 쓰려는 것이다. 쓰던 자리로 돌려보낸다 —
			// 여기서 삼키면 답을 통째로 허공에 치게 된다.
			return ui.CmdStay, nil
		}
	}
}
