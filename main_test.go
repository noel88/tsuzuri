package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
	"github.com/noel88/tsuzuri/internal/ui"
)

func writeProblem(t *testing.T, dir string, p pack.Problem) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "packs"), 0o755); err != nil {
		t.Fatal(err)
	}
	name := string(p.Dir) + ".jsonl"
	if err := store.Append(filepath.Join(dir, "packs", name), p); err != nil {
		t.Fatal(err)
	}
}

// ko2ja 팩이 없어도 「1. 드릴 시작」이 동작해야 한다.
// 키를 방향 인덱스로 매기면 첫 세트가 42가 되어 이 메뉴가 죽는다.
func TestFirstPackKeyMatchesMenuShortcut(t *testing.T) {
	dir := t.TempDir()
	writeProblem(t, dir, pack.Problem{
		ID: "p001", Dir: pack.JaToKo, Level: "N2", Topic: "뉴스",
		Prompt: "昨日カフェに行った。", Reference: "어제 카페에 갔다.",
		Style: pack.StylePlain,
	})

	a := &app{dataDir: dir}
	sets, err := a.loadPackSets()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 {
		t.Fatalf("세트 개수 = %d, 기대 1", len(sets))
	}

	key, ok := ui.ParseMenuKey("1")
	if !ok {
		t.Fatal("1은 종료가 아니다")
	}
	if _, found := findSet(sets, key); !found {
		t.Errorf("「1. 드릴 시작」이 첫 세트(%q)에 닿지 않는다 — 키가 %q로 매겨졌다",
			key, sets[0].key)
	}
}

func TestPackSetKeysAreContiguous(t *testing.T) {
	dir := t.TempDir()
	writeProblem(t, dir, pack.Problem{
		ID: "k1", Dir: pack.KoToJa, Prompt: "a", Reference: "b", Style: pack.StylePlain,
	})
	writeProblem(t, dir, pack.Problem{
		ID: "j1", Dir: pack.JaToKo, Prompt: "c", Reference: "d", Style: pack.StylePlain,
	})

	a := &app{dataDir: dir}
	sets, err := a.loadPackSets()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"41", "42"}
	for i, s := range sets {
		if s.key != want[i] {
			t.Errorf("%d번째 세트 키 = %q, 기대 %q", i, s.key, want[i])
		}
	}
}
