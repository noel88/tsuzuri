package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noel88/tsuzuri/internal/gen"
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
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

	// 「1. 드릴 시작」은 자료실 첫 팩이다. 그 뜻은 dispatch가 안다 —
	// 번호를 쳐서 오든 화살표로 골라서 오든 같은 곳으로 가야 하므로,
	// 입력을 읽는 자리에서 번호를 바꾸지 않는다.
	if _, found := findSet(sets, "1"); found {
		t.Error("「1」이 팩 번호로 잡힌다. 그러면 메뉴 항목과 팩이 겹친다")
	}
	if sets[0].key != "41" {
		t.Errorf("첫 세트 = %q, 기대 \"41\"", sets[0].key)
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

func TestShareSplitsCountAcrossDirections(t *testing.T) {
	// 홀수는 앞쪽이 하나 더 가진다.
	if got := []int{share(9, 2, 0), share(9, 2, 1)}; got[0] != 5 || got[1] != 4 {
		t.Errorf("9문항을 둘로 = %v, 기대 [5 4]", got)
	}
	// 한 방향이면 그대로 간다.
	if got := share(50, 1, 0); got != 50 {
		t.Errorf("share = %d, 기대 50", got)
	}
	// 0문항짜리 호출은 돈만 쓰고 빈 팩을 만든다.
	if got := share(1, 2, 1); got != 1 {
		t.Errorf("share = %d, 최소 한 문항은 줘야 한다", got)
	}
}

func TestPackProgressCountsDistinctProblems(t *testing.T) {
	// 팩은 다 풀어도 자료실에서 사라지지 않는다. 어디까지 왔는지를
	// 문항 수 자리에 적어 주지 않으면 화면만 봐서는 알 수 없다.
	set := packSet{problems: []pack.Problem{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}}

	if got := set.progress(nil); got != "0/3" {
		t.Errorf("아직 안 푼 팩 = %q", got)
	}
	// 같은 문제를 여러 번 풀어도 하나로 센다.
	if got := set.progress(map[string]bool{"p1": true, "p2": true}); got != "2/3" {
		t.Errorf("progress = %q, 기대 \"2/3\"", got)
	}
	if got := set.progress(map[string]bool{"p1": true, "p2": true, "p3": true}); got != "3/3" {
		t.Errorf("다 푼 팩 = %q", got)
	}
	// 다른 팩의 문제는 세지 않는다.
	if got := set.progress(map[string]bool{"다른팩/p1": true}); got != "0/3" {
		t.Errorf("progress = %q — 다른 팩을 셌다", got)
	}
}

func TestArchiveFoldsFinishedPacks(t *testing.T) {
	// 팩은 지우지 않으므로 쌓이기만 한다. 다 푼 것을 접어 두지 않으면
	// 오늘 할 것이 어느 줄인지 찾기 어려워진다.
	sets := []packSet{
		{key: "41", dir: pack.KoToJa, problems: []pack.Problem{{ID: "a1"}, {ID: "a2"}}},
		{key: "42", dir: pack.JaToKo, problems: []pack.Problem{{ID: "b1"}}},
	}
	solved := map[string]bool{"a1": true, "a2": true} // 41만 끝냈다

	a := &app{}
	got := a.archive(sets, solved)
	if len(got) != 2 {
		t.Fatalf("줄 %d개: %+v", len(got), got)
	}
	if got[0].Key != "42" {
		t.Errorf("끝낸 팩이 접히지 않았다: %+v", got)
	}
	if got[1].Key != doneToggleKey || got[1].Value != "1팩" {
		t.Errorf("접어 둔 것을 알리지 않았다: %+v", got[1])
	}

	// 펼치면 다시 보인다. 지운 것이 아니라 접어 둔 것이다.
	a.showDone = true
	got = a.archive(sets, solved)
	if len(got) != 3 {
		t.Fatalf("펼친 줄 %d개: %+v", len(got), got)
	}
	if got[0].Key != "41" || got[0].Value != "완료" {
		t.Errorf("완료 표시가 없다: %+v", got[0])
	}
}

func TestArchiveHasNoToggleWhenNothingFinished(t *testing.T) {
	a := &app{}
	sets := []packSet{{key: "41", problems: []pack.Problem{{ID: "a1"}}}}
	if got := a.archive(sets, nil); len(got) != 1 {
		t.Errorf("끝낸 팩이 없는데 줄이 늘었다: %+v", got)
	}
}

func TestArchiveShowsEverythingWhenAllDone(t *testing.T) {
	// 전부 끝냈는데 접으면 자료실이 통째로 비고, 「드릴 시작」이 가리킬
	// 줄이 없어진다.
	a := &app{}
	sets := []packSet{{key: "41", problems: []pack.Problem{{ID: "a1"}}}}
	got := a.archive(sets, map[string]bool{"a1": true})
	if len(got) == 0 || got[0].Key != "41" {
		t.Errorf("전부 끝냈더니 자료실이 비었다: %+v", got)
	}
}

func TestBatchCountSplitsLargeRequests(t *testing.T) {
	// 한 번에 다 만들 수는 없다. 장문이 섞이면 한 문항이 길어서 응답
	// 한도를 넘기고, 넘기면 몇 분을 기다린 끝에 끊긴 채 토큰만 청구된다.
	for n, want := range map[int]int{0: 0, 1: 1, 25: 1, 26: 2, 150: 6, 300: 12} {
		if got := batchCount(n); got != want {
			t.Errorf("batchCount(%d) = %d, 기대 %d", n, got, want)
		}
	}
}

func TestExtendablePicksMatchingPackWithRoom(t *testing.T) {
	sets := []packSet{
		{key: "41", dir: pack.KoToJa, source: "a", problems: []pack.Problem{{ID: "p1", Level: "N3"}}},
		{key: "42", dir: pack.KoToJa, source: "b", problems: []pack.Problem{{ID: "p2", Level: "N2"}}},
		{key: "43", dir: pack.JaToKo, source: "c", problems: []pack.Problem{{ID: "p3", Level: "N2"}}},
	}

	got, ok := extendable(sets, pack.KoToJa, "N2")
	if !ok || got.key != "42" {
		t.Errorf("이어 받을 팩 = %+v, %v", got, ok)
	}
	// 레벨이 다르면 고르지 않는다.
	if _, ok := extendable(sets, pack.KoToJa, "N1"); ok {
		t.Error("레벨이 다른 팩을 골랐다")
	}
	// 레벨이 섞인 팩도 고르지 않는다 — 어느 레벨에 이어 받는지 알 수 없다.
	mixed := []packSet{{key: "44", dir: pack.KoToJa, problems: []pack.Problem{
		{ID: "p1", Level: "N2"}, {ID: "p2", Level: "N3"},
	}}}
	if _, ok := extendable(mixed, pack.KoToJa, "N2"); ok {
		t.Error("레벨이 섞인 팩을 골랐다")
	}
}

func TestExtendableSkipsFullPacks(t *testing.T) {
	full := make([]pack.Problem, gen.MaxPack)
	for i := range full {
		full[i] = pack.Problem{ID: "p", Level: "N2"}
	}
	sets := []packSet{{key: "41", dir: pack.KoToJa, problems: full}}
	if _, ok := extendable(sets, pack.KoToJa, "N2"); ok {
		t.Error("꽉 찬 팩에 이어 받으려 한다")
	}
}

func TestExtendableSkipsPacksWithBlankLevels(t *testing.T) {
	// levels()는 빈 값을 건너뛰므로 레벨이 비어 있는 문항이 섞여도
	// 「단일 레벨」로 보인다. 그런 팩에 이어 받으면 라벨이 거짓말을 한다.
	sets := []packSet{{key: "41", dir: pack.KoToJa, problems: []pack.Problem{
		{ID: "p1", Level: "N2"}, {ID: "p2", Level: ""},
	}}}
	if _, ok := extendable(sets, pack.KoToJa, "N2"); ok {
		t.Error("레벨이 빈 문항이 섞인 팩을 골랐다")
	}
}
