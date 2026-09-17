package progress

import (
	"testing"
	"time"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/store"
	tsync "github.com/noel88/tsuzuri/internal/sync"
)

var base = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

func at(days int) time.Time { return base.AddDate(0, 0, days) }

func attempt(id, problem string, when time.Time) store.Attempt {
	return store.Attempt{ID: id, PackID: problem, At: when}
}

func graded(attemptID string, failed bool) tsync.Feedback {
	f := tsync.Feedback{AttemptID: attemptID}
	if failed {
		f.Notes = []tsync.Note{{Span: "は", Why: "조사가 틀렸다", Level: tsync.LevelError}}
	} else {
		f.Notes = []tsync.Note{{Span: "とても", Why: "더 자연스러운 선택이 있다", Level: tsync.LevelNuance}}
	}
	return f
}

func find(t *testing.T, cards []Card, id string) Card {
	t.Helper()
	for _, c := range cards {
		if c.ProblemID == id {
			return c
		}
	}
	t.Fatalf("카드가 없다: %s (전체 %d장)", id, len(cards))
	return Card{}
}

func TestCardsIgnoresProblemsWithoutErrors(t *testing.T) {
	// 첨삭이 nuance만 달고 왔으면 틀린 것이 아니다. 복습 목록에 들어가면
	// 맞은 답을 틀린 것으로 다루는 셈이 된다 (스펙 §5.4).
	cards := Cards(
		[]store.Attempt{attempt("a1", "p1", at(0))},
		[]tsync.Feedback{graded("a1", false)},
		nil, at(1))

	if len(cards) != 0 {
		t.Errorf("복습 대상이 아닌데 %d장이 들어왔다: %+v", len(cards), cards)
	}
}

func TestCardsEntersOnFeedbackError(t *testing.T) {
	cards := Cards(
		[]store.Attempt{attempt("a1", "p1", at(0))},
		[]tsync.Feedback{graded("a1", true)},
		nil, at(0).Add(time.Hour))

	c := find(t, cards, "p1")
	if c.State != StateDue {
		t.Errorf("state = %v, 틀린 직후엔 바로 풀 차례여야 한다", c.State)
	}
	if c.Reason != ReasonError {
		t.Errorf("reason = %q", c.Reason)
	}
}

func TestCardsEntersOnUserFlag(t *testing.T) {
	// 첨삭이 조용해도 사용자가 다시 보겠다고 하면 들어온다.
	cards := Cards(
		[]store.Attempt{attempt("a1", "p1", at(0))},
		[]tsync.Feedback{graded("a1", false)},
		[]Mark{{ProblemID: "p1", At: at(0).Add(time.Minute), Kind: KindFlag}},
		at(1))

	c := find(t, cards, "p1")
	if c.State != StateDue || c.Reason != ReasonFlag {
		t.Errorf("표시한 문제가 %v/%q", c.State, c.Reason)
	}
}

func TestCardsWaitsForFeedbackInsteadOfRepeating(t *testing.T) {
	// 복습으로 답은 냈는데 첨삭이 아직 안 왔다. 판정할 근거가 없으므로
	// 그대로 두면 안 된다 — 그대로 두면 같은 문제가 계속 다시 나온다.
	cards := Cards(
		[]store.Attempt{
			attempt("a1", "p1", at(0)),
			attempt("a2", "p1", at(1)),
		},
		[]tsync.Feedback{graded("a1", true)},
		nil, at(2))

	c := find(t, cards, "p1")
	if c.State != StateWaiting {
		t.Errorf("state = %v, 첨삭 대기여야 한다", c.State)
	}
	if got := Due(cards); len(got) != 0 {
		t.Errorf("첨삭 대기 중인데 다시 출제된다: %v", got)
	}
}

func TestCardsSchedulesByGrowingIntervals(t *testing.T) {
	attempts := []store.Attempt{attempt("a1", "p1", at(0))}
	feedback := []tsync.Feedback{graded("a1", true)}

	// 맞힐 때마다 다음 간격으로 물러난다.
	for i, want := range Intervals {
		id := string(rune('b' + i))
		when := at(10 * (i + 1))
		attempts = append(attempts, attempt(id, "p1", when))
		feedback = append(feedback, graded(id, false))

		cards := Cards(attempts, feedback, nil, when.Add(time.Minute))
		c := find(t, cards, "p1")
		if c.State != StateScheduled {
			t.Fatalf("%d번째: state = %v, 쉬는 중이어야 한다", i+1, c.State)
		}
		if got := c.Due.Sub(when); got != want {
			t.Errorf("%d번째 간격 = %v, 기대 %v", i+1, got, want)
		}
	}

	// 마지막 간격까지 지나면 졸업이고 더 나오지 않는다.
	last := at(100)
	attempts = append(attempts, attempt("z", "p1", last))
	feedback = append(feedback, graded("z", false))

	cards := Cards(attempts, feedback, nil, last.AddDate(0, 0, 30))
	if c := find(t, cards, "p1"); c.State != StateGraduated {
		t.Errorf("state = %v, 졸업이어야 한다", c.State)
	}
	if got := Due(cards); len(got) != 0 {
		t.Errorf("졸업한 문제가 다시 출제된다: %v", got)
	}
}

func TestCardsResetsAfterAnotherError(t *testing.T) {
	// 한 번 맞혀 간격이 늘어난 뒤에 다시 틀리면 처음으로 돌아간다.
	cards := Cards(
		[]store.Attempt{
			attempt("a1", "p1", at(0)),
			attempt("a2", "p1", at(1)),
			attempt("a3", "p1", at(5)),
		},
		[]tsync.Feedback{graded("a1", true), graded("a2", false), graded("a3", true)},
		nil, at(5).Add(time.Hour))

	c := find(t, cards, "p1")
	if c.Streak != 0 {
		t.Errorf("streak = %d, 다시 틀렸으면 0이어야 한다", c.Streak)
	}
	if c.State != StateDue {
		t.Errorf("state = %v, 바로 풀 차례여야 한다", c.State)
	}
}

func TestCardsFlagOverridesSchedule(t *testing.T) {
	// 쉬는 중인 문제라도 사용자가 표시하면 바로 올라온다.
	cards := Cards(
		[]store.Attempt{
			attempt("a1", "p1", at(0)),
			attempt("a2", "p1", at(1)),
		},
		[]tsync.Feedback{graded("a1", true), graded("a2", false)},
		[]Mark{{ProblemID: "p1", At: at(2), Kind: KindFlag}},
		at(2).Add(time.Hour))

	c := find(t, cards, "p1")
	if c.State != StateDue {
		t.Errorf("state = %v, 표시했으면 바로 나와야 한다", c.State)
	}
}

func TestCardsOrdersOldestFirst(t *testing.T) {
	cards := Cards(
		[]store.Attempt{
			attempt("a1", "p1", at(3)),
			attempt("a2", "p2", at(1)),
		},
		[]tsync.Feedback{graded("a1", true), graded("a2", true)},
		nil, at(5))

	if got := Due(cards); len(got) != 2 || got[0] != "p2" {
		t.Errorf("오래 밀린 것부터 나와야 한다: %v", got)
	}
}

func TestSummarizeCountsStreakAndMissing(t *testing.T) {
	now := at(10)
	attempts := []store.Attempt{
		{ID: "a1", PackID: "p1", At: at(8), Analysis: analyze.Analysis{Missing: []string{"つもりだ", "ばかり"}}},
		{ID: "a2", PackID: "p2", At: at(9), Analysis: analyze.Analysis{Missing: []string{"つもりだ"}}},
		{ID: "a3", PackID: "p2", At: at(10), Analysis: analyze.Analysis{Missing: []string{"つもりだ"}}},
	}

	s := Summarize(attempts, nil, nil, now)

	if s.Total != 3 || s.Problems != 2 || s.Days != 3 {
		t.Errorf("total=%d problems=%d days=%d", s.Total, s.Problems, s.Days)
	}
	if s.Streak != 3 {
		t.Errorf("streak = %d, 사흘 연속이다", s.Streak)
	}
	if len(s.TopMissing) == 0 || s.TopMissing[0].Text != "つもりだ" || s.TopMissing[0].N != 3 {
		t.Errorf("가장 자주 빠뜨린 표현이 틀렸다: %+v", s.TopMissing)
	}
}

func TestSummarizeKeepsYesterdayStreak(t *testing.T) {
	// 오늘 아직 안 풀었다고 어제까지 쌓은 것이 사라지면, 그것을 지키려고
	// 억지로 푸는 일이 생긴다. 이 앱은 그런 압박을 주지 않는다.
	attempts := []store.Attempt{
		{ID: "a1", PackID: "p1", At: at(8)},
		{ID: "a2", PackID: "p1", At: at(9)},
	}

	if got := Summarize(attempts, nil, nil, at(10)).Streak; got != 2 {
		t.Errorf("streak = %d, 어제까지 이틀이 이어져 있다", got)
	}
	// 이틀을 건너뛰면 끊긴다.
	if got := Summarize(attempts, nil, nil, at(11)).Streak; got != 0 {
		t.Errorf("streak = %d, 끊겼어야 한다", got)
	}
}

func TestSummarizeReadsAnalysisFromFile(t *testing.T) {
	// 파일에서 읽은 분석 결과는 map 이다. 방금 만든 것과 같게 다뤄야 한다.
	attempts := []store.Attempt{
		{ID: "a1", PackID: "p1", At: at(0), Analysis: map[string]any{
			"missing": []any{"ておく"},
		}},
	}

	s := Summarize(attempts, nil, nil, at(0))
	if len(s.TopMissing) != 1 || s.TopMissing[0].Text != "ておく" {
		t.Errorf("저장된 분석에서 표현을 못 꺼냈다: %+v", s.TopMissing)
	}
}

func TestLastPack(t *testing.T) {
	// 파일 순서가 아니라 시각으로 고른다.
	got, ok := LastPack([]store.Attempt{
		attempt("a1", "p9", at(5)),
		attempt("a2", "p2", at(1)),
	})
	if !ok || got != "p9" {
		t.Errorf("LastPack = %q, %v", got, ok)
	}
	if _, ok := LastPack(nil); ok {
		t.Error("푼 것이 없으면 이어할 곳도 없다")
	}
}

func TestCardsStopWaitingForFeedbackThatNeverComes(t *testing.T) {
	// 첨삭이 영영 오지 않는 일이 있다 — sync가 영구 실패로 판정해 큐에서
	// 빼거나 답안을 못 찾아 버리면 그렇다. 그대로 두면 그 문제는 복습
	// 드릴에 다시는 안 나오는데, 사용자는 왜 안 나오는지 알 수 없다.
	attempts := []store.Attempt{
		attempt("a1", "p1", at(0)),
		attempt("a2", "p1", at(1)), // 첨삭이 오지 않는다
	}
	feedback := []tsync.Feedback{graded("a1", true)}

	// 한도 안에서는 기다린다.
	if c := find(t, Cards(attempts, feedback, nil, at(5)), "p1"); c.State != StateWaiting {
		t.Errorf("state = %v, 아직 기다려야 한다", c.State)
	}

	// 한도가 지나면 다시 낸다.
	late := at(1).Add(pendingTTL + time.Hour)
	cards := Cards(attempts, feedback, nil, late)
	if c := find(t, cards, "p1"); c.State != StateDue {
		t.Errorf("state = %v, 다시 낼 차례여야 한다", c.State)
	}
	if got := Due(cards); len(got) != 1 {
		t.Errorf("Due = %v, 다시 나와야 한다", got)
	}
}
