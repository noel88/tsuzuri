package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
	tsync "github.com/noel88/tsuzuri/internal/sync"
)

func at() time.Time { return time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC) }

func seed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "packs"), 0o755); err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(store.Append(filepath.Join(dir, "packs", "p.jsonl"), pack.Problem{
		ID: "p001", Dir: pack.KoToJa, Level: "N3", Topic: "일상",
		Prompt: "어제 카페에 갔다.", Reference: "昨日カフェに行った。",
		Style: pack.StylePlain,
	}))
	must(store.Append(filepath.Join(dir, "attempts.jsonl"), store.Attempt{
		ID: "a001", PackID: "p001", At: at(), Answer: "昨日カフェは静かくて。",
	}))
	must(store.Append(filepath.Join(dir, "feedback.jsonl"), tsync.Feedback{
		AttemptID: "a001", At: at(), Corrected: "昨日カフェは静かで。",
		Notes: []tsync.Note{
			{Span: "静かくて", Why: "な형용사는 「静かで」", Level: tsync.LevelError},
			{Span: "カフェ", Why: "「喫茶店」도 자연스럽다", Level: tsync.LevelNuance},
		},
		Overall: "활용 하나만 고치면 됩니다.",
	}))
	return dir
}

func TestWriteProducesPlainTextNote(t *testing.T) {
	dir := seed(t)
	path, n, err := Write(dir, at())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 1 {
		t.Errorf("건수 = %d, 기대 1", n)
	}
	if filepath.Ext(path) != ".txt" {
		t.Errorf("순정 포메라가 읽을 .txt여야 한다: %q", path)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"어제 카페에 갔다", "昨日カフェは静かくて", "昨日カフェは静かで",
		"昨日カフェに行った", "な형용사", "활용 하나만",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("노트에 %q가 없다:\n%s", want, s)
		}
	}
}

func TestWriteDistinguishesErrorFromNuance(t *testing.T) {
	dir := seed(t)
	path, _, err := Write(dir, at())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	if !strings.Contains(s, "* 静かくて") {
		t.Errorf("오류는 *로 표시해야 한다:\n%s", s)
	}
	if !strings.Contains(s, "- カフェ") {
		t.Errorf("뉘앙스는 -로 표시해야 한다:\n%s", s)
	}
}

func TestWriteUsesCRLFForPomera(t *testing.T) {
	// 순정 포메라는 Windows 계열 텍스트를 다룬다.
	dir := seed(t)
	path, _, err := Write(dir, at())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "\r\n") {
		t.Error("CRLF 줄바꿈이어야 한다")
	}
	if strings.Contains(strings.ReplaceAll(string(b), "\r\n", ""), "\n") {
		t.Error("LF만 있는 줄이 섞이면 안 된다")
	}
}

func TestWriteNoFeedbackIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	path, n, err := Write(dir, at())
	if err != nil {
		t.Fatalf("첨삭이 없어도 오류가 아니다: %v", err)
	}
	if n != 0 || path != "" {
		t.Errorf("파일을 만들면 안 된다: %q %d", path, n)
	}
}

func TestWriteSkipsOrphanedFeedback(t *testing.T) {
	// 답안이 없는 첨삭은 보여줄 맥락이 없다.
	dir := t.TempDir()
	if err := store.Append(filepath.Join(dir, "feedback.jsonl"), tsync.Feedback{
		AttemptID: "없는답안", At: at(), Overall: "총평",
	}); err != nil {
		t.Fatal(err)
	}
	_, n, err := Write(dir, at())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("건수 = %d, 기대 0", n)
	}
}
