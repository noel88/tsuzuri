package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noel88/tsuzuri/internal/llm"
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
)

func fixedNow() time.Time { return time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC) }

const feedbackReply = `{"corrected":"昨日カフェは静かで。",
 "notes":[{"span":"静かくて","why":"な형용사는 「静かで」","level":"error"},
          {"span":"ずっと","why":"틀리진 않지만 「長く」가 더 가깝다","level":"nuance"}],
 "overall":"활용 오류 하나를 빼면 의미는 전달됩니다."}`

func writeJSONL(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(path, v); err != nil {
		t.Fatal(err)
	}
}

// 큐에 한 건이 들어 있는 데이터 디렉터리를 만든다.
func seed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "packs", "p.jsonl"), pack.Problem{
		ID: "p001", Dir: pack.KoToJa, Level: "N3", Topic: "일상",
		Prompt: "어제 카페에 갔다.", Reference: "昨日カフェに行った。",
		KeyPoints: []string{"カフェ"}, Style: pack.StylePlain,
	})
	at := fixedNow()
	writeJSONL(t, filepath.Join(dir, "attempts.jsonl"), store.Attempt{
		ID: "a001", PackID: "p001", At: at, Answer: "昨日カフェは静かくて。",
	})
	writeJSONL(t, filepath.Join(dir, "queue.jsonl"), store.QueueItem{
		AttemptID: "a001", At: at,
	})
	return dir
}

func TestRunWritesFeedback(t *testing.T) {
	dir := seed(t)
	f := &llm.FakeClient{Reply: feedbackReply}

	res, err := Run(context.Background(), f, dir, fixedNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Processed != 1 {
		t.Fatalf("처리 건수 = %d, 기대 1", res.Processed)
	}

	got, err := store.ReadAll[Feedback](filepath.Join(dir, "feedback.jsonl"))
	if err != nil {
		t.Fatalf("feedback 읽기: %v", err)
	}
	if len(got) != 1 || got[0].AttemptID != "a001" {
		t.Fatalf("첨삭이 attempt를 가리켜야 한다: %+v", got)
	}
	if got[0].Corrected != "昨日カフェは静かで。" {
		t.Errorf("corrected = %q", got[0].Corrected)
	}
	if len(got[0].Notes) != 2 {
		t.Fatalf("note 개수 = %d, 기대 2", len(got[0].Notes))
	}
	if got[0].Notes[0].Level != LevelError || got[0].Notes[1].Level != LevelNuance {
		t.Errorf("오류와 뉘앙스를 구분해야 한다: %+v", got[0].Notes)
	}
}

func TestRunSendsContext(t *testing.T) {
	dir := seed(t)
	f := &llm.FakeClient{Reply: feedbackReply}
	if _, err := Run(context.Background(), f, dir, fixedNow); err != nil {
		t.Fatal(err)
	}
	body := f.Got.System + f.Got.User
	for _, want := range []string{"어제 카페에 갔다", "昨日カフェに行った", "静かくて"} {
		if !strings.Contains(body, want) {
			t.Errorf("제시문·참조·답안이 모두 전달되어야 한다. %q 없음", want)
		}
	}
}

func TestRunPromptForbidsPenalizingDifference(t *testing.T) {
	// 스펙 §5.4를 온라인 쪽에서도 유지한다.
	dir := seed(t)
	f := &llm.FakeClient{Reply: feedbackReply}
	if _, err := Run(context.Background(), f, dir, fixedNow); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Got.System, "다르다는 이유") {
		t.Errorf("참조와 다르다는 이유로 지적하지 말 것을 명시해야 한다:\n%s", f.Got.System)
	}
	if !strings.Contains(f.Got.System, "점수를 매기지") {
		t.Errorf("점수 금지를 명시해야 한다:\n%s", f.Got.System)
	}
}

func TestRunEmptiesQueue(t *testing.T) {
	dir := seed(t)
	f := &llm.FakeClient{Reply: feedbackReply}
	if _, err := Run(context.Background(), f, dir, fixedNow); err != nil {
		t.Fatal(err)
	}
	q, err := store.ReadAll[store.QueueItem](filepath.Join(dir, "queue.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 0 {
		t.Errorf("처리한 항목은 큐에서 빠져야 한다: %+v", q)
	}
}

func TestRunKeepsFailedItemsInQueue(t *testing.T) {
	// 네트워크가 한 번 흔들렸다고 그 답안이 영영 첨삭받지 못하면 안 된다.
	dir := seed(t)
	f := &llm.FakeClient{Err: errors.New("연결 끊김")}

	res, err := Run(context.Background(), f, dir, fixedNow)
	if err != nil {
		t.Fatalf("개별 실패는 전체 오류가 아니다: %v", err)
	}
	if res.Processed != 0 || res.Failed != 1 {
		t.Errorf("결과 = %+v, 기대 {0 1 0}", res)
	}
	q, _ := store.ReadAll[store.QueueItem](filepath.Join(dir, "queue.jsonl"))
	if len(q) != 1 {
		t.Errorf("실패한 항목은 큐에 남아야 한다: %+v", q)
	}
}

func TestRunSkipsAlreadyFeedbacked(t *testing.T) {
	dir := seed(t)
	f := &llm.FakeClient{Reply: feedbackReply}
	if _, err := Run(context.Background(), f, dir, fixedNow); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := f.Calls

	// 큐에 같은 항목을 다시 넣어도 재전송하면 안 된다 — 두 배 과금이다.
	writeJSONL(t, filepath.Join(dir, "queue.jsonl"), store.QueueItem{
		AttemptID: "a001", At: fixedNow(),
	})
	res, err := Run(context.Background(), f, dir, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if res.Processed != 0 {
		t.Errorf("이미 첨삭받은 건을 다시 보내면 안 된다: %d건", res.Processed)
	}
	if f.Calls != callsAfterFirst {
		t.Errorf("API를 다시 부르면 안 된다: %d → %d", callsAfterFirst, f.Calls)
	}
}

func TestRunEmptyQueueIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	f := &llm.FakeClient{Reply: feedbackReply}
	res, err := Run(context.Background(), f, dir, fixedNow)
	if err != nil {
		t.Fatalf("빈 큐는 오류가 아니다: %v", err)
	}
	if res.Processed != 0 {
		t.Errorf("처리 건수 = %d", res.Processed)
	}
	if f.Calls != 0 {
		t.Error("빈 큐에 API를 부르면 안 된다")
	}
}

func TestRunPrioritizesFlaggedItems(t *testing.T) {
	dir := seed(t)
	at := fixedNow()
	writeJSONL(t, filepath.Join(dir, "attempts.jsonl"), store.Attempt{
		ID: "a002", PackID: "p001", At: at, Answer: "두번째",
	})
	writeJSONL(t, filepath.Join(dir, "queue.jsonl"), store.QueueItem{
		AttemptID: "a002", Priority: true, At: at,
	})

	f := &llm.FakeClient{Reply: feedbackReply}
	if _, err := Run(context.Background(), f, dir, fixedNow); err != nil {
		t.Fatal(err)
	}
	got, _ := store.ReadAll[Feedback](filepath.Join(dir, "feedback.jsonl"))
	if len(got) != 2 {
		t.Fatalf("둘 다 처리되어야 한다: %d", len(got))
	}
	if got[0].AttemptID != "a002" {
		t.Errorf("우선 표시된 항목이 먼저여야 한다: %q", got[0].AttemptID)
	}
}

func TestRunKeepsAnswerWhenPackIsMissing(t *testing.T) {
	// 팩 파일이 지워지거나 이름이 바뀌면(SD카드를 PC에 꽂는 전제) 문제를
	// 찾을 수 없다. 사용자가 쓴 답안이므로 큐에서 버리면 안 된다.
	dir := seed(t)
	if err := os.Remove(filepath.Join(dir, "packs", "p.jsonl")); err != nil {
		t.Fatal(err)
	}

	f := &llm.FakeClient{Reply: feedbackReply}
	res, err := Run(context.Background(), f, dir, fixedNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Missing != 1 {
		t.Errorf("문제를 못 찾은 건수 = %d, 기대 1", res.Missing)
	}
	q, _ := store.ReadAll[store.QueueItem](filepath.Join(dir, "queue.jsonl"))
	if len(q) != 1 {
		t.Errorf("답안이 큐에 남아야 한다 — 지우면 영영 첨삭받지 못한다: %+v", q)
	}
	if f.Calls != 0 {
		t.Error("문제 없이 API를 부르면 안 된다")
	}
}

func TestRunDropsOrphanedQueueItems(t *testing.T) {
	// 답안이 없는 큐 항목은 첨삭할 근거가 없다. 큐에서 빠져야 한다.
	dir := seed(t)
	writeJSONL(t, filepath.Join(dir, "queue.jsonl"), store.QueueItem{
		AttemptID: "없는답안", At: fixedNow(),
	})
	f := &llm.FakeClient{Reply: feedbackReply}
	if _, err := Run(context.Background(), f, dir, fixedNow); err != nil {
		t.Fatal(err)
	}
	q, _ := store.ReadAll[store.QueueItem](filepath.Join(dir, "queue.jsonl"))
	if len(q) != 0 {
		t.Errorf("고아 항목이 큐에 남아 있다: %+v", q)
	}
}

func TestRunLeavesNoTempFile(t *testing.T) {
	dir := seed(t)
	f := &llm.FakeClient{Reply: feedbackReply}
	if _, err := Run(context.Background(), f, dir, fixedNow); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "queue.jsonl.tmp")); !os.IsNotExist(err) {
		t.Error("임시 파일이 남으면 안 된다")
	}
}
