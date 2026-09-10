package drill

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
)

func fixedNow() time.Time {
	return time.Date(2026, 9, 10, 14, 22, 3, 0, time.UTC)
}

func problems() []pack.Problem {
	return []pack.Problem{
		{
			ID: "p001", Dir: pack.KoToJa, Level: "N3", Topic: "일상",
			Prompt: "어제 카페에 갔다.", Reference: "昨日カフェに行った。",
			KeyPoints: []string{"カフェ"}, Style: pack.StylePlain,
		},
		{
			ID: "p002", Dir: pack.KoToJa, Level: "N3", Topic: "일상",
			Prompt: "비가 온다.", Reference: "雨が降る。",
			KeyPoints: []string{"雨"}, Style: pack.StylePlain,
		},
	}
}

func newSession(t *testing.T, dir, input string) (*Session, *bytes.Buffer) {
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
		In:       bufio.NewReader(strings.NewReader(input)),
		Out:      &out,
		TermW:    92,
		Now:      fixedNow,
	}, &out
}

func TestRunSavesAttemptAndQueuesFeedback(t *testing.T) {
	dir := t.TempDir()
	s, _ := newSession(t, dir, "昨日カフェに行った。\nx\n")

	got, err := s.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != OutcomeQuit {
		t.Errorf("Outcome = %v, 기대 OutcomeQuit", got)
	}

	attempts, err := store.ReadAll[store.Attempt](filepath.Join(dir, "attempts.jsonl"))
	if err != nil {
		t.Fatalf("attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempt 개수 = %d, 기대 1", len(attempts))
	}
	if attempts[0].Answer != "昨日カフェに行った。" || attempts[0].PackID != "p001" {
		t.Errorf("attempt = %+v", attempts[0])
	}
	if attempts[0].Analysis == nil {
		t.Error("분석 결과가 함께 저장되어야 한다")
	}

	queue, err := store.ReadAll[store.QueueItem](filepath.Join(dir, "queue.jsonl"))
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("모든 답안이 큐에 적재되어야 한다: %d", len(queue))
	}
	if queue[0].AttemptID != attempts[0].ID {
		t.Errorf("큐가 attempt를 가리켜야 한다: %q vs %q", queue[0].AttemptID, attempts[0].ID)
	}
	if queue[0].Priority {
		t.Error("F를 안 눌렀으므로 Priority가 false여야 한다")
	}
}

func TestRunPrioritySetsQueueFlag(t *testing.T) {
	dir := t.TempDir()
	s, _ := newSession(t, dir, "昨日カフェに行った。\nf\n비가 온다\nx\n")
	if _, err := s.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	queue, _ := store.ReadAll[store.QueueItem](filepath.Join(dir, "queue.jsonl"))
	if len(queue) == 0 || !queue[0].Priority {
		t.Errorf("F 입력 시 Priority가 true여야 한다: %+v", queue)
	}
}

func TestRunRetryKeepsPreviousAttempt(t *testing.T) {
	dir := t.TempDir()
	// 첫 답 → r(다시) → 둘째 답 → x
	s, _ := newSession(t, dir, "まちがい\nr\n昨日カフェに行った。\nx\n")
	if _, err := s.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	attempts, _ := store.ReadAll[store.Attempt](filepath.Join(dir, "attempts.jsonl"))
	if len(attempts) != 2 {
		t.Fatalf("두 attempt가 모두 남아야 한다: %d", len(attempts))
	}
	if attempts[0].PackID != attempts[1].PackID {
		t.Error("같은 문제를 다시 푼 것이어야 한다")
	}
}

func TestRunHidesReferenceUntilAnswered(t *testing.T) {
	dir := t.TempDir()
	s, out := newSession(t, dir, "昨日カフェに行った。\nx\n")
	if _, err := s.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	text := out.String()
	pi := strings.Index(text, "어제 카페에 갔다")
	ri := strings.Index(text, "참조:")
	if pi < 0 || ri < 0 {
		t.Fatalf("제시문과 참조가 모두 출력되어야 한다:\n%s", text)
	}
	if ri < pi {
		t.Error("참조는 제시문 뒤에 나와야 한다")
	}
}

func TestRunReturnsMenuOutcome(t *testing.T) {
	dir := t.TempDir()
	s, _ := newSession(t, dir, "昨日カフェに行った。\nm\n")
	got, err := s.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != OutcomeMenu {
		t.Errorf("Outcome = %v, 기대 OutcomeMenu", got)
	}
}

func TestRunStopsWhenInputExhausted(t *testing.T) {
	dir := t.TempDir()
	// 입력이 도중에 끊긴다. 무한 루프에 빠지면 안 된다.
	s, _ := newSession(t, dir, "昨日カフェに行った。\n")
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := s.Run(); err != nil {
			t.Errorf("Run: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("입력이 끊겼는데 종료하지 않았다 — 무한 루프")
	}
}

func TestRunFinishesAllProblems(t *testing.T) {
	dir := t.TempDir()
	s, _ := newSession(t, dir, "昨日カフェに行った。\n\n雨が降る。\n\n")
	got, err := s.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != OutcomeDone {
		t.Errorf("Outcome = %v, 기대 OutcomeDone", got)
	}
	attempts, _ := store.ReadAll[store.Attempt](filepath.Join(dir, "attempts.jsonl"))
	if len(attempts) != 2 {
		t.Errorf("두 문제를 모두 풀어야 한다: %d", len(attempts))
	}
}
