// Package sync는 쌓인 답안의 첨삭을 받아온다.
//
// 표준 라이브러리 sync와 이름이 같다. 이 프로젝트는 둘을 한 파일에서
// 쓰지 않으며, 임포트하는 쪽은 tsync 별칭을 쓴다.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/noel88/tsuzuri/internal/llm"
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
)

// 지적의 수준.
const (
	LevelError  = "error"  // 문법·활용·어휘가 실제로 틀린 것
	LevelNuance = "nuance" // 틀리지는 않았지만 더 나은 선택이 있는 것
)

// Note는 첨삭의 지적 하나다.
type Note struct {
	Span  string `json:"span"`
	Why   string `json:"why"`
	Level string `json:"level"`
}

// Feedback은 답안 하나에 대한 첨삭이다.
type Feedback struct {
	AttemptID string    `json:"attempt_id"`
	At        time.Time `json:"at"`
	Corrected string    `json:"corrected"`
	Notes     []Note    `json:"notes"`
	Overall   string    `json:"overall"`
}

const systemPrompt = `당신은 한국어 화자의 일본어·한국어 번역 작문을 첨삭합니다.

학습자가 제시문을 보고 목표 언어로 작문했습니다. 참조 번역도 함께 드립니다.

지켜야 할 것:

- **참조와 다르다는 이유로 지적하지 마세요.** 참조는 모범 예시 하나일 뿐이고
  번역은 정답이 여럿입니다. 다른 표현으로 같은 뜻을 정확히 전달했다면
  그것은 옳은 답입니다.
- 지적은 두 종류로 구분하세요.
  - level "error": 문법·활용·어휘가 실제로 틀린 것
  - level "nuance": 틀리지는 않았지만 더 나은 선택이 있는 것
- span은 문제가 되는 부분을 학습자의 답안에서 그대로 인용하세요.
- overall은 두 문장 이내의 총평입니다. 점수를 매기지 마세요.
- corrected는 학습자의 답안을 최소한으로 고친 것입니다.
  참조 번역으로 갈아치우지 말고, 학습자가 쓴 표현을 살리세요.`

var feedbackSchema = map[string]any{
	"corrected": map[string]any{"type": "string"},
	"notes": map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"span":  map[string]any{"type": "string"},
				"why":   map[string]any{"type": "string"},
				"level": map[string]any{"type": "string", "enum": []string{LevelError, LevelNuance}},
			},
			"required":             []string{"span", "why", "level"},
			"additionalProperties": false,
		},
	},
	"overall": map[string]any{"type": "string"},
}

// Result는 한 번의 sync 결과다.
type Result struct {
	Processed int  // 첨삭을 받은 건수
	Failed    int  // 일시적 실패로 큐에 남긴 건수
	Missing   int  // 문제를 찾지 못해 큐에 남긴 건수
	Dropped   int  // 다시 시도해도 같을 실패라 큐에서 뺀 건수
	Aborted   bool // 연속 실패로 중단했는지
}

// maxConsecutiveFailures는 이만큼 연달아 실패하면 나머지를 포기한다.
//
// Wi-Fi가 끊긴 채로 큐에 쌓인 100건을 계속 시도하면, 화면에는 "받아오는
// 중입니다..."만 떠 있고 빠져나갈 길은 Ctrl+C뿐이다. 호출마다 시간도
// 비용도 든다.
const maxConsecutiveFailures = 3

// permanent는 다시 시도해도 결과가 같을 실패인지 본다.
//
// 토큰 한도에 걸린 답안을 그대로 큐에 두면 sync할 때마다 같은 금액을
// 다시 내고 같은 곳에서 실패한다.
func permanent(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, sign := range []string{"토큰 한도", "거부되었습니다", "구조화된 응답"} {
		if strings.Contains(msg, sign) {
			return true
		}
	}
	return false
}

// Run은 큐에 쌓인 답안의 첨삭을 받아 feedback.jsonl에 append한다.
func Run(ctx context.Context, c llm.Client, dataDir string, now func() time.Time) (Result, error) {
	if now == nil {
		now = time.Now
	}
	queuePath := filepath.Join(dataDir, "queue.jsonl")
	feedbackPath := filepath.Join(dataDir, "feedback.jsonl")

	queue, err := store.ReadAll[store.QueueItem](queuePath)
	if err != nil {
		return Result{}, err
	}
	if len(queue) == 0 {
		return Result{}, nil
	}

	attempts, err := store.ReadAll[store.Attempt](filepath.Join(dataDir, "attempts.jsonl"))
	if err != nil {
		return Result{}, err
	}
	byID := make(map[string]store.Attempt, len(attempts))
	for _, a := range attempts {
		byID[a.ID] = a
	}

	done, err := store.ReadAll[Feedback](feedbackPath)
	if err != nil {
		return Result{}, err
	}
	// 이미 첨삭받은 답안을 다시 보내면 그대로 두 배 과금이다.
	already := make(map[string]bool, len(done))
	for _, f := range done {
		already[f.AttemptID] = true
	}

	problems, _, err := pack.ByID(filepath.Join(dataDir, "packs"))
	if err != nil {
		return Result{}, err
	}

	// 같은 답안이 큐에 여러 줄 있을 수 있다. 우선 표시는 나중에 한 줄 더
	// 남기는 방식이기 때문이다. 답안 단위로 합치지 않으면 같은 답안을
	// 두 번 보내 두 배로 과금된다.
	pending := make([]store.QueueItem, 0, len(queue))
	seen := map[string]int{}
	for _, q := range queue {
		if already[q.AttemptID] {
			continue
		}
		if i, dup := seen[q.AttemptID]; dup {
			if q.Priority {
				pending[i].Priority = true
			}
			continue
		}
		seen[q.AttemptID] = len(pending)
		pending = append(pending, q)
	}
	// 우선 표시된 항목을 먼저 처리한다.
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].Priority && !pending[j].Priority
	})

	processed, missing, dropped := 0, 0, 0
	consecutive := 0
	aborted := false
	var failed []store.QueueItem
	for i, q := range pending {
		if aborted {
			// 남은 항목은 손대지 않고 큐에 그대로 둔다.
			failed = append(failed, pending[i:]...)
			break
		}
		a, ok := byID[q.AttemptID]
		if !ok {
			// 답안 자체가 없으면 첨삭할 대상이 없다. 큐에서 버린다.
			continue
		}
		p, ok := problems[a.PackID]
		if !ok {
			// 문제를 찾을 수 없다 — 팩 파일이 지워졌거나 이름이 바뀌었다.
			// SD카드를 PC에 꽂는 것을 전제한 설계이므로 충분히 일어난다.
			// 사용자가 쓴 답안이므로 버리지 않고 큐에 남긴다. 팩을 되돌리면
			// 다음 sync에서 첨삭된다.
			failed = append(failed, q)
			missing++
			continue
		}
		fb, err := review(ctx, c, p, a, now())
		if err != nil {
			if permanent(err) {
				// 다시 보내도 같은 곳에서 같은 금액을 잃는다. 큐에서 뺀다.
				dropped++
				consecutive = 0
				continue
			}
			// 일시적 실패는 큐에 남긴다. 네트워크가 한 번 흔들렸다고
			// 그 답안이 영영 첨삭받지 못하면 안 된다.
			failed = append(failed, q)
			consecutive++
			if consecutive >= maxConsecutiveFailures {
				aborted = true
			}
			continue
		}
		consecutive = 0
		if err := store.Append(feedbackPath, fb); err != nil {
			return Result{Processed: processed}, err
		}
		processed++
	}

	res := Result{
		Processed: processed,
		Failed:    len(failed) - missing,
		Missing:   missing,
		Dropped:   dropped,
		Aborted:   aborted,
	}
	if err := rewriteQueue(queuePath, failed); err != nil {
		return res, err
	}
	return res, nil
}

func review(ctx context.Context, c llm.Client, p pack.Problem, a store.Attempt, at time.Time) (Feedback, error) {
	user := fmt.Sprintf(
		"방향: %s\n레벨: %s\n요구 문체: %s\n\n제시문: %s\n참조 번역: %s\n\n학습자의 답안: %s",
		p.Dir, p.Level, p.Style, p.Prompt, p.Reference, a.Answer)

	raw, err := c.Complete(ctx, llm.Request{
		System:    systemPrompt,
		User:      user,
		ToolName:  "emit_feedback",
		ToolDesc:  "학습자의 답안에 대한 첨삭을 제출합니다.",
		Schema:    feedbackSchema,
		Required:  []string{"corrected", "notes", "overall"},
		MaxTokens: 8000,
	})
	if err != nil {
		return Feedback{}, err
	}

	var out struct {
		Corrected string `json:"corrected"`
		Notes     []Note `json:"notes"`
		Overall   string `json:"overall"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return Feedback{}, fmt.Errorf("첨삭 응답을 해석하지 못했습니다: %w", err)
	}
	return Feedback{
		AttemptID: a.ID,
		At:        at,
		Corrected: out.Corrected,
		Notes:     out.Notes,
		Overall:   out.Overall,
	}, nil
}

// rewriteQueue는 큐를 남은 항목만으로 다시 쓴다.
//
// append-only 원칙의 유일한 예외다. 임시 파일에 쓰고 rename해서,
// 도중에 전원이 꺼져도 큐가 반쯤 쓰인 상태로 남지 않게 한다.
func rewriteQueue(path string, remaining []store.QueueItem) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, q := range remaining {
		if err := enc.Encode(q); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
