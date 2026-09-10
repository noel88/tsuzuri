package store

import (
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
