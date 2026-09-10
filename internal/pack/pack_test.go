package pack

import (
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
	got, err := LoadDir("../../testdata/packs", KoToJa)
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
