package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendThenReadAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.jsonl")

	a1 := Attempt{ID: "a001", PackID: "p001", At: time.Now(), Answer: "첫번째"}
	a2 := Attempt{ID: "a002", PackID: "p002", At: time.Now(), Answer: "두번째"}

	if err := Append(path, a1); err != nil {
		t.Fatalf("첫 Append: %v", err)
	}
	if err := Append(path, a2); err != nil {
		t.Fatalf("둘째 Append: %v", err)
	}

	got, err := ReadAll[Attempt](path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("개수 = %d, 기대 2", len(got))
	}
	if got[0].ID != "a001" || got[1].ID != "a002" {
		t.Errorf("순서가 보존되어야 한다: %q, %q", got[0].ID, got[1].ID)
	}
	if got[1].Answer != "두번째" {
		t.Errorf("Answer = %q", got[1].Answer)
	}
}

func TestReadAllMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "없는파일.jsonl")
	got, err := ReadAll[Attempt](path)
	if err != nil {
		t.Fatalf("없는 파일은 오류가 아니어야 한다: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("빈 슬라이스여야 한다, 길이 %d", len(got))
	}
}

func TestReadAllTolerantOfTruncatedLastLine(t *testing.T) {
	// 포메라가 갑자기 꺼져 마지막 줄이 잘린 상황을 흉내낸다.
	path := filepath.Join(t.TempDir(), "attempts.jsonl")
	if err := Append(path, Attempt{ID: "a001", PackID: "p001"}); err != nil {
		t.Fatal(err)
	}
	if err := appendRaw(path, `{"id":"a002","pack_i`); err != nil {
		t.Fatal(err)
	}

	got, err := ReadAll[Attempt](path)
	if err != nil {
		t.Fatalf("잘린 마지막 줄은 무시되어야 한다: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a001" {
		t.Errorf("온전한 줄만 남아야 한다: %+v", got)
	}
}

func TestReadAllRejectsCorruptionInMiddle(t *testing.T) {
	// 마지막 줄이 아닌 곳이 손상됐다면 조용히 넘어가면 안 된다.
	path := filepath.Join(t.TempDir(), "attempts.jsonl")
	if err := appendRaw(path, `{"id":"a001"`); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, Attempt{ID: "a002"}); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadAll[Attempt](path); err == nil {
		t.Error("중간 손상은 오류로 보고되어야 한다")
	}
}

func TestQueueItemRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.jsonl")
	if err := Append(path, QueueItem{AttemptID: "a001", Priority: true, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAll[QueueItem](path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 1 || got[0].AttemptID != "a001" || !got[0].Priority {
		t.Errorf("QueueItem 왕복 실패: %+v", got)
	}
}

// 전원이 끊겨 줄이 잘린 뒤에도 파일이 계속 쓸 수 있어야 한다.
//
// 조각에 다음 기록이 들러붙으면 그 줄은 더 이상 마지막 줄이 아니게 되고,
// ReadAll이 파일 전체를 거부해 앱이 시작조차 못 한다.
func TestAppendHealsTornLastLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.jsonl")
	if err := Append(path, QueueItem{AttemptID: "a001"}); err != nil {
		t.Fatal(err)
	}
	if err := appendRaw(path, `{"attempt_id":"a002","prio`); err != nil {
		t.Fatal(err)
	}
	// 조각은 개행 없이 끝난다. 여기서 전원이 끊긴 상황이다.
	if err := truncateFinalNewline(path); err != nil {
		t.Fatal(err)
	}

	if err := Append(path, QueueItem{AttemptID: "a003"}); err != nil {
		t.Fatalf("잘린 파일에도 이어 쓸 수 있어야 한다: %v", err)
	}
	if err := Append(path, QueueItem{AttemptID: "a004"}); err != nil {
		t.Fatal(err)
	}

	got, err := ReadAll[QueueItem](path)
	if err != nil {
		t.Fatalf("이후 읽기가 실패하면 앱이 시작조차 못 한다: %v", err)
	}
	var ids []string
	for _, q := range got {
		ids = append(ids, q.AttemptID)
	}
	want := []string{"a001", "a003", "a004"}
	if len(ids) != len(want) {
		t.Fatalf("항목 = %v, 기대 %v (잘린 a002만 사라져야 한다)", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("항목 = %v, 기대 %v", ids, want)
			break
		}
	}
}

func TestAppendHealsFileWithNoNewlineAtAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.jsonl")
	if err := os.WriteFile(path, []byte(`{"id":"a001"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, Attempt{ID: "a002"}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAll[Attempt](path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a002" {
		t.Errorf("조각만 버리고 새 기록은 남아야 한다: %+v", got)
	}
}

// truncateFinalNewline은 파일 끝의 개행 하나를 지워 잘린 쓰기를 흉내낸다.
func truncateFinalNewline(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return os.WriteFile(path, b, 0o644)
}
