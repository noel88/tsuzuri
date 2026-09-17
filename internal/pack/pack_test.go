package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadParsesEachLine(t *testing.T) {
	got, err := Load("../../testdata/packs/sample-ko2ja.jsonl")
	if err != nil {
		t.Fatalf("Load() 오류: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("문제 개수 = %d, 기대 2", len(got))
	}
	p := got[0]
	if p.ID != "p001" {
		t.Errorf("ID = %q, 기대 \"p001\"", p.ID)
	}
	if p.Dir != KoToJa {
		t.Errorf("Dir = %q, 기대 %q", p.Dir, KoToJa)
	}
	if p.Style != StylePlain {
		t.Errorf("Style = %q, 기대 %q", p.Style, StylePlain)
	}
	if len(p.KeyPoints) != 3 {
		t.Errorf("KeyPoints 개수 = %d, 기대 3", len(p.KeyPoints))
	}
	if got[1].Style != StylePolite {
		t.Errorf("두 번째 Style = %q, 기대 %q", got[1].Style, StylePolite)
	}
}

func TestLoadSkipsBlankLines(t *testing.T) {
	got, err := Load("../../testdata/packs/sample-ko2ja.jsonl")
	if err != nil {
		t.Fatalf("빈 줄 때문에 실패하면 안 된다: %v", err)
	}
	for i, p := range got {
		if p.ID == "" {
			t.Errorf("%d번째 문제의 ID가 비어 있다 — 빈 줄이 파싱된 것", i)
		}
	}
}

func TestLoadReportsLineNumberOnBadJSON(t *testing.T) {
	_, err := Load("../../testdata/malformed/broken.jsonl")
	if err == nil {
		t.Fatal("깨진 JSON에서 오류가 나야 한다")
	}
	if !strings.Contains(err.Error(), "2번째 줄") {
		t.Errorf("오류에 줄 번호가 있어야 한다: %v", err)
	}
}

func TestLoadDirFiltersByDirection(t *testing.T) {
	got, _, err := LoadDir("../../testdata/packs", KoToJa)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	for _, p := range got {
		if p.Dir != KoToJa {
			t.Errorf("%s의 방향이 %q — ko2ja만 나와야 한다", p.ID, p.Dir)
		}
	}
	if len(got) == 0 {
		t.Error("ko2ja 문제가 하나도 없다")
	}
}

func TestByIDDetectsCollisionAcrossPacks(t *testing.T) {
	// 모델은 팩마다 p001부터 번호를 매긴다. ID가 겹치면 답안이 어느 문제
	// 것인지 알 수 없어 엉뚱한 문제로 채점하고 그 비용을 청구받는다.
	dir := t.TempDir()
	line := `{"id":"p001","dir":"ko2ja","prompt":"질문","reference":"답","style":"plain"}` + "\n"
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, skipped, err := ByID(dir)
	if err != nil {
		t.Fatalf("겹친다고 앱이 멈추면 안 된다: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("먼저 읽은 팩만 남아야 한다: %v", got)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "겹칩니다") {
		t.Fatalf("겹친 팩을 건너뛰었다고 알려야 한다: %+v", skipped)
	}
}

// id가 겹치는 팩을 드릴에서만 허용하면, 사용자가 겹친 팩을 지운 뒤 남은
// 팩의 문제로 채점되어 엉뚱한 첨삭에 값을 치르게 된다. 드릴에서도 뺀다.
func TestLoadDirSkipsPacksWithCollidingIDs(t *testing.T) {
	dir := t.TempDir()
	line := `{"id":"p001","dir":"ko2ja","prompt":"질문","reference":"답","style":"plain"}` + "\n"
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, skipped, err := LoadDir(dir, KoToJa)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("겹친 문제가 드릴에 두 번 나오면 안 된다: %+v", got)
	}
	if len(skipped) != 1 {
		t.Errorf("건너뛴 팩을 알려야 한다: %+v", skipped)
	}
}

func TestByIDLoadsAcrossDirections(t *testing.T) {
	got, _, err := ByID("../../testdata/packs")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("문제가 없다")
	}
	if _, ok := got["p001"]; !ok {
		t.Errorf("p001을 찾지 못했다: %v", got)
	}
}

// 맥에서 SD카드에 팩을 복사하면 macOS가 AppleDouble 파일을 같이 만든다.
// 그 파일을 팩으로 읽으려다 실패하면 앱이 시작조차 못 했다.
func TestLoadDirSkipsAppleDoubleSidecars(t *testing.T) {
	dir := t.TempDir()
	good := `{"id":"p001","dir":"ko2ja","prompt":"질문","reference":"답","style":"plain"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "pack.jsonl"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	// AppleDouble은 바이너리라 첫 바이트부터 JSON이 아니다.
	if err := os.WriteFile(filepath.Join(dir, "._pack.jsonl"), []byte{0, 5, 22, 7, 0, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte{0, 1}, 0o644); err != nil {
		t.Fatal(err)
	}

	got, skipped, err := LoadDir(dir, KoToJa)
	if err != nil {
		t.Fatalf("사이드카 파일 때문에 실패하면 안 된다: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("정상 팩은 읽혀야 한다: %+v", got)
	}
	if len(skipped) != 0 {
		t.Errorf("사이드카는 조용히 건너뛴다(경고 대상 아님): %+v", skipped)
	}
}

func TestLoadDirKeepsGoingWhenOnePackIsBroken(t *testing.T) {
	// 팩 하나가 전원 차단으로 잘렸거나 손으로 고치다 깨져도, 나머지로 계속
	// 쓸 수 있어야 한다. 대신 무엇을 건너뛰었는지는 알려야 한다.
	dir := t.TempDir()
	good := `{"id":"p001","dir":"ko2ja","prompt":"질문","reference":"답","style":"plain"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "a-good.jsonl"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b-broken.jsonl"), []byte("{이건 JSON이 아니다\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, skipped, err := LoadDir(dir, KoToJa)
	if err != nil {
		t.Fatalf("깨진 팩 하나로 앱이 못 켜지면 안 된다: %v", err)
	}
	if len(got) != 1 || got[0].ID != "p001" {
		t.Errorf("정상 팩은 읽혀야 한다: %+v", got)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Path, "b-broken") {
		t.Fatalf("건너뛴 팩을 알려야 한다: %+v", skipped)
	}
	if skipped[0].Reason == "" {
		t.Error("왜 건너뛰었는지 알려야 한다")
	}
}

func TestByIDSkipsSidecarsAndBrokenPacks(t *testing.T) {
	dir := t.TempDir()
	good := `{"id":"p001","dir":"ko2ja","prompt":"질문","reference":"답","style":"plain"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "good.jsonl"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "._good.jsonl"), []byte{0, 5}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte("깨짐\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, skipped, err := ByID(dir)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if _, ok := got["p001"]; !ok {
		t.Errorf("정상 문제를 찾지 못했다: %v", got)
	}
	if len(skipped) != 1 {
		t.Errorf("깨진 팩 하나만 보고해야 한다: %+v", skipped)
	}
}

func TestLoadStripsUTF8BOM(t *testing.T) {
	// 손으로 만든 팩에 BOM이 붙으면 첫 줄이 통째로 깨졌다.
	dir := t.TempDir()
	path := filepath.Join(dir, "bom.jsonl")
	line := "\ufeff" + `{"id":"p001","dir":"ko2ja","prompt":"질문","reference":"답","style":"plain"}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("BOM 때문에 실패하면 안 된다: %v", err)
	}
	if len(got) != 1 || got[0].ID != "p001" {
		t.Errorf("Load = %+v", got)
	}
}

func TestLoadNormalizesLevelAndRecordsSource(t *testing.T) {
	// 실기에서 level = "2" 로 50문항을 받은 적이 있다. 자료실에 "2" 로
	// 보이면 사용자는 자기가 고른 N2가 무시됐다고 읽는다.
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	line := `{"id":"x1","dir":"ko2ja","level":"2","topic":"가족","prompt":"가","reference":"あ"}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ps, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if ps[0].Level != "N2" {
		t.Errorf("level = %q, 기대 \"N2\"", ps[0].Level)
	}
	if ps[0].Source != path {
		t.Errorf("source = %q, 어느 팩에서 왔는지 알아야 자료실이 팩 단위로 보인다", ps[0].Source)
	}
}

func TestNormalizeLevelLeavesEverythingElse(t *testing.T) {
	for in, want := range map[string]string{
		"2": "N2", "5": "N5", "N3": "N3", "초급": "초급", "": "", "12": "12", "6": "6",
	} {
		if got := NormalizeLevel(in); got != want {
			t.Errorf("NormalizeLevel(%q) = %q, 기대 %q", in, got, want)
		}
	}
}

func TestLengthOfSortsPromptsIntoThreeKinds(t *testing.T) {
	// 포메라 화면은 한 줄에 한글 약 60자다. 단문 한 줄, 중문 다섯 줄,
	// 장문 스무 줄이라는 감각을 글자 수로 옮긴 것이다.
	short := "비가 올 것 같아서 우산을 가져왔습니다."
	medium := strings.Repeat("어제 처음 간 카페가 생각보다 조용해서 오래 앉아 있었다. ", 3)
	long := strings.Repeat("요즘 계속 바빠서 방 청소를 할 기력도 없다. ", 15)

	for prompt, want := range map[string]string{short: LenShort, medium: LenMedium, long: LenLong} {
		if got := LengthOf(prompt); got != want {
			t.Errorf("%d자 = %q, 기대 %q", len([]rune(prompt)), got, want)
		}
	}
	if !(Problem{Prompt: long}).IsLong() {
		t.Error("장문인데 한 줄 입력으로 받으려 한다")
	}
	if (Problem{Prompt: short}).IsLong() {
		t.Error("단문에 에디터를 연다")
	}
}
