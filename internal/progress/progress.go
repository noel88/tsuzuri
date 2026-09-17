// Package progress는 지금까지 푼 것에서 진도와 복습 목록을 계산한다.
//
// 이 패키지는 상태를 거의 저장하지 않는다. 복습 목록도 통계도 이미 있는
// attempts.jsonl·feedback.jsonl 에서 다시 계산하며, 새로 쓰는 파일은
// marks.jsonl 하나뿐이다. 사용자가 직접 남긴 표시만 그 파일에 들어간다.
//
// 그렇게 한 이유는 두 가지다. 계산된 상태를 따로 저장하면 원본과 어긋날
// 수 있고 — 전원이 예고 없이 끊기는 기기에서는 실제로 어긋난다 — 계산
// 규칙을 고칠 때마다 저장된 상태를 마이그레이션해야 한다.
package progress

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/store"
	tsync "github.com/noel88/tsuzuri/internal/sync"
)

// Mark는 사용자가 문제에 남긴 표시다 (marks.jsonl).
type Mark struct {
	ProblemID string    `json:"problem_id"`
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
}

// KindFlag는 「다시 볼 것」 표시다. 결과 화면에서 B로 남긴다.
const KindFlag = "flag"

// Intervals는 복습 간격이다. 맞힐 때마다 다음 칸으로 간다.
//
// 마지막 간격까지 지나면 졸업이고 더 나오지 않는다. 간격을 늘려 가며
// 같은 것을 다시 만나는 것이 요점이라, 정확한 값보다 「점점 뜸해진다」가
// 중요하다.
var Intervals = []time.Duration{
	24 * time.Hour,
	3 * 24 * time.Hour,
	7 * 24 * time.Hour,
}

// pendingTTL은 첨삭을 기다려 주는 한도다.
//
// 답은 냈는데 첨삭이 오지 않는 일이 있다. sync가 영구 실패로 판정해 큐에서
// 빼거나, 답안을 못 찾아 버리면 그 첨삭은 영영 생기지 않는다. 그때 카드가
// 「기다리는 중」에 갇히면 복습 드릴이 그 문제를 다시는 내주지 않는데,
// 사용자는 왜 안 나오는지 알 길이 없다. 한도가 지나면 다시 낼 차례로 본다 —
// 한 번 더 푸는 것이 영영 안 나오는 것보다 낫다.
const pendingTTL = 14 * 24 * time.Hour

// State는 복습 카드가 지금 어느 상태인지다.
type State int

const (
	StateDue       State = iota // 지금 풀 차례
	StateWaiting                // 답했고 첨삭을 기다린다
	StateScheduled              // 다음 날짜까지 쉰다
	StateGraduated              // 졸업 — 더 나오지 않는다
)

// Card는 복습 대상 문제 하나다.
type Card struct {
	ProblemID string
	State     State
	Due       time.Time
	Streak    int    // 연달아 맞힌 횟수
	Reason    string // 왜 복습 목록에 들어왔는지
}

// 복습 목록에 들어오는 이유.
const (
	ReasonError = "첨삭 오류"
	ReasonFlag  = "표시"
)

// Cards는 복습 목록을 계산한다. 졸업한 것도 함께 돌려준다 — 통계에 쓴다.
//
// 맞혔는지 틀렸는지는 사용자에게 묻지 않는다. 첨삭에 error 지적이 있으면
// 틀린 것이고 없으면 맞힌 것이다. 스스로 채점하게 하면 그것이 곧 점수가
// 되는데, 이 앱은 점수를 매기지 않는다 (스펙 §5.4). 대신 첨삭이 올 때까지는
// 판정을 미룬다 — 그 동안 그 문제는 다시 나오지 않는다.
func Cards(attempts []store.Attempt, feedback []tsync.Feedback, marks []Mark, now time.Time) []Card {
	fb := make(map[string]tsync.Feedback, len(feedback))
	for _, f := range feedback {
		fb[f.AttemptID] = f
	}

	// 문제별로 사건을 모은다.
	type event struct {
		at     time.Time
		flag   bool // 사용자의 표시
		graded bool // 첨삭이 도착한 답안
		failed bool // 그 첨삭에 error 지적이 있었나
	}
	byProblem := make(map[string][]event)

	for _, at := range attempts {
		e := event{at: at.At}
		if f, ok := fb[at.ID]; ok {
			e.graded = true
			e.failed = hasError(f)
		}
		byProblem[at.PackID] = append(byProblem[at.PackID], e)
	}
	for _, m := range marks {
		if m.Kind != KindFlag {
			continue
		}
		byProblem[m.ProblemID] = append(byProblem[m.ProblemID], event{at: m.At, flag: true})
	}

	var out []Card
	for id, events := range byProblem {
		sort.SliceStable(events, func(i, j int) bool { return events[i].at.Before(events[j].at) })

		var (
			seeded    bool
			card      Card
			pending   bool      // 마지막 답안의 첨삭이 아직 안 왔다
			pendingAt time.Time // 그 답안을 낸 때
		)
		card.ProblemID = id

		for _, e := range events {
			switch {
			case e.flag:
				// 표시는 언제든 카드를 처음으로 되돌린다. 사용자가
				// 「이건 다시 봐야겠다」고 말한 것이므로 일정보다 우선한다.
				seeded, pending = true, false
				card.Reason, card.Streak, card.Due = ReasonFlag, 0, e.at

			case !seeded:
				// 아직 복습 대상이 아니다. 틀린 것이 확인돼야 들어온다.
				if e.graded && e.failed {
					seeded = true
					card.Reason, card.Streak, card.Due = ReasonError, 0, e.at
				}

			case !e.graded:
				pending, pendingAt = true, e.at

			case e.failed:
				pending = false
				card.Streak, card.Due = 0, e.at

			default:
				pending = false
				card.Streak++
				if card.Streak <= len(Intervals) {
					card.Due = e.at.Add(Intervals[card.Streak-1])
				}
			}
		}
		if !seeded {
			continue
		}

		// 너무 오래 기다린 것은 기다리기를 그만둔다.
		if pending && now.Sub(pendingAt) > pendingTTL {
			pending = false
		}

		switch {
		case card.Streak > len(Intervals):
			card.State = StateGraduated
		case pending:
			card.State = StateWaiting
		case !card.Due.After(now):
			card.State = StateDue
		default:
			card.State = StateScheduled
		}
		out = append(out, card)
	}

	// 오래 밀린 것부터. 같으면 문제 번호순 — 같은 목록은 매번 같게 보여야 한다.
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Due.Equal(out[j].Due) {
			return out[i].Due.Before(out[j].Due)
		}
		return out[i].ProblemID < out[j].ProblemID
	})
	return out
}

// Due는 지금 풀 차례인 문제의 ID만 순서대로 돌려준다.
func Due(cards []Card) []string {
	var out []string
	for _, c := range cards {
		if c.State == StateDue {
			out = append(out, c.ProblemID)
		}
	}
	return out
}

// hasError는 첨삭에 실제로 틀렸다는 지적이 있는지 본다.
// nuance는 틀린 것이 아니므로 복습 대상으로 삼지 않는다.
func hasError(f tsync.Feedback) bool {
	for _, n := range f.Notes {
		if n.Level == tsync.LevelError {
			return true
		}
	}
	return false
}

// Count는 표현 하나와 그 횟수다.
type Count struct {
	Text string
	N    int
}

// Stats는 진도 화면에 보여 줄 수치다.
type Stats struct {
	Total      int // 제출한 답안 수
	Problems   int // 손댄 문제 수 (같은 문제를 여러 번 푼 것은 하나)
	Days       int // 푼 날 수
	Streak     int // 연속으로 푼 날
	Feedback   int // 받은 첨삭 수
	Due        int
	Waiting    int
	Graduated  int
	TopMissing []Count // 자주 빠뜨리는 표현
}

// Summarize는 통계를 낸다. now는 연속 일수를 오늘 기준으로 세기 위해 받는다.
func Summarize(attempts []store.Attempt, feedback []tsync.Feedback, cards []Card, now time.Time) Stats {
	s := Stats{Total: len(attempts), Feedback: len(feedback)}

	problems := make(map[string]bool)
	days := make(map[string]bool)
	missing := make(map[string]int)
	for _, at := range attempts {
		problems[at.PackID] = true
		days[at.At.Format("2006-01-02")] = true
		for _, m := range missingOf(at.Analysis) {
			missing[m]++
		}
	}
	s.Problems = len(problems)
	s.Days = len(days)
	s.Streak = streak(days, now)
	s.TopMissing = top(missing, 5)

	for _, c := range cards {
		switch c.State {
		case StateDue:
			s.Due++
		case StateWaiting:
			s.Waiting++
		case StateGraduated:
			s.Graduated++
		}
	}
	return s
}

// streak는 오늘(또는 어제)부터 거꾸로 이어진 날 수를 센다.
//
// 어제까지만 이어져 있어도 끊긴 것으로 보지 않는다. 오늘 아직 안 풀었다고
// 해서 어제까지 쌓은 것이 사라진 것처럼 보이면, 그것을 지키려고 억지로
// 한 문제를 푸는 일이 생긴다. 그런 압박은 이 앱이 하려는 것이 아니다.
func streak(days map[string]bool, now time.Time) int {
	day := now
	if !days[day.Format("2006-01-02")] {
		day = day.AddDate(0, 0, -1)
	}
	n := 0
	for days[day.Format("2006-01-02")] {
		n++
		day = day.AddDate(0, 0, -1)
	}
	return n
}

// top은 많이 나온 것부터 n개를 돌려준다.
func top(m map[string]int, n int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{Text: k, N: v})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Text < out[j].Text
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// missingOf는 저장된 분석 결과에서 빠뜨린 표현을 꺼낸다.
//
// store.Attempt.Analysis 는 any 다 — 저장소는 분석 결과의 형태를 모른다.
// 파일에서 읽으면 map 이고 방금 만든 것이면 analyze.Analysis 라, 양쪽을
// 같게 다루려고 JSON 을 한 번 거친다. 답안 수백 건 규모에서 충분히 싸다.
func missingOf(v any) []string {
	if a, ok := v.(analyze.Analysis); ok {
		return a.Missing
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var a analyze.Analysis
	if err := json.Unmarshal(b, &a); err != nil {
		return nil
	}
	return a.Missing
}

// LastPack은 마지막으로 답한 문제의 ID를 돌려준다. 이어하기에 쓴다.
func LastPack(attempts []store.Attempt) (string, bool) {
	if len(attempts) == 0 {
		return "", false
	}
	last := attempts[0]
	for _, at := range attempts[1:] {
		if at.At.After(last.At) {
			last = at
		}
	}
	return last.PackID, true
}
