package gen

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noel88/tsuzuri/internal/llm"
	"github.com/noel88/tsuzuri/internal/pack"
)

const goodReply = `{"problems":[
 {"id":"x1","dir":"ko2ja","level":"N3","topic":"일상",
  "prompt":"어제 카페에 갔다.","reference":"昨日カフェに行った。",
  "key_points":["カフェ"],"traps":["「行く」의 과거형"],"style":"plain"},
 {"id":"x2","dir":"ko2ja","level":"N3","topic":"일상",
  "prompt":"비가 온다.","reference":"雨が降る。",
  "key_points":["雨"],"traps":[],"style":"plain"}
]}`

func spec() Spec {
	return Spec{Dir: pack.KoToJa, Level: "N3", Topic: "일상", Count: 2}
}

func at() time.Time { return time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC) }

func TestGenerateParsesProblems(t *testing.T) {
	f := &llm.FakeClient{Reply: goodReply}
	got, err := Generate(context.Background(), f, spec())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("문제 개수 = %d, 기대 2", len(got))
	}
	if got[0].Reference != "昨日カフェに行った。" {
		t.Errorf("reference = %q", got[0].Reference)
	}
	if len(got[0].KeyPoints) == 0 {
		t.Error("key_points가 있어야 오프라인 채점이 된다")
	}
}

func TestGeneratePassesSpecToPrompt(t *testing.T) {
	f := &llm.FakeClient{Reply: goodReply}
	if _, err := Generate(context.Background(), f, spec()); err != nil {
		t.Fatal(err)
	}
	body := f.Got.User + f.Got.System
	for _, want := range []string{"N3", "일상", "ko2ja"} {
		if !strings.Contains(body, want) {
			t.Errorf("프롬프트에 %q가 들어가야 한다", want)
		}
	}
	if f.Got.ToolName == "" || len(f.Got.Schema) == 0 {
		t.Error("구조화된 출력을 요청해야 한다")
	}
	if len(f.Got.Required) == 0 {
		t.Error("required를 지정해야 한다")
	}
}

func TestGeneratePromptForbidsTreatingReferenceAsOnlyAnswer(t *testing.T) {
	// 스펙 §5.4. 생성 단계에서부터 이 전제를 심는다.
	f := &llm.FakeClient{Reply: goodReply}
	if _, err := Generate(context.Background(), f, spec()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Got.System, "유일한 정답") {
		t.Errorf("참조가 유일한 정답이 아님을 명시해야 한다:\n%s", f.Got.System)
	}
}

func TestGenerateRejectsWrongDirection(t *testing.T) {
	f := &llm.FakeClient{Reply: strings.ReplaceAll(goodReply, `"ko2ja"`, `"ja2ko"`)}
	if _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("요청한 방향과 다르면 거부해야 한다")
	}
}

func TestGenerateRejectsEmptyReference(t *testing.T) {
	f := &llm.FakeClient{Reply: `{"problems":[{"id":"x1","dir":"ko2ja","prompt":"질문","reference":"","style":"plain"}]}`}
	if _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("참조가 비면 드릴이 성립하지 않는다")
	}
}

func TestGenerateRejectsDuplicateIDs(t *testing.T) {
	f := &llm.FakeClient{Reply: strings.ReplaceAll(goodReply, `"x2"`, `"x1"`)}
	if _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("id가 겹치면 답안이 어느 문제 것인지 알 수 없다")
	}
}

func TestGenerateRejectsMalformedJSON(t *testing.T) {
	f := &llm.FakeClient{Reply: `이건 JSON이 아니다`}
	if _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("깨진 응답은 오류여야 한다")
	}
}

func TestGenerateRejectsEmptyResult(t *testing.T) {
	f := &llm.FakeClient{Reply: `{"problems":[]}`}
	if _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("빈 결과는 오류여야 한다")
	}
}

func TestGenerateRejectsZeroCount(t *testing.T) {
	f := &llm.FakeClient{Reply: goodReply}
	if _, err := Generate(context.Background(), f, Spec{Dir: pack.KoToJa, Count: 0}); err == nil {
		t.Error("문항 수 0은 오류여야 한다")
	}
	if f.Calls != 0 {
		t.Error("잘못된 요청으로 API를 부르면 안 된다 — 비용이다")
	}
}

func TestWritePackProducesLoadableFile(t *testing.T) {
	dir := t.TempDir()
	ps := []pack.Problem{{
		ID: "x1", Dir: pack.KoToJa, Level: "N3", Topic: "일상",
		Prompt: "어제 카페에 갔다.", Reference: "昨日カフェに行った。",
		KeyPoints: []string{"カフェ"}, Style: pack.StylePlain,
	}}

	path, err := WritePack(dir, spec(), ps, at())
	if err != nil {
		t.Fatalf("WritePack: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("팩 디렉터리에 써야 한다: %q", path)
	}

	// 나중에 읽을 바로 그 코드로 확인한다.
	got, err := pack.Load(path)
	if err != nil {
		t.Fatalf("생성한 팩을 pack.Load가 읽지 못한다: %v", err)
	}
	if len(got) != 1 || got[0].ID != "x1" {
		t.Errorf("왕복 실패: %+v", got)
	}
	if got[0].Reference != "昨日カフェに行った。" {
		t.Errorf("일본어가 깨졌다: %q", got[0].Reference)
	}
}

func TestWritePackDoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	ps := []pack.Problem{{ID: "x1", Dir: pack.KoToJa, Prompt: "a", Reference: "b", Style: pack.StylePlain}}

	p1, err := WritePack(dir, spec(), ps, at())
	if err != nil {
		t.Fatal(err)
	}
	p2, err := WritePack(dir, spec(), ps, at())
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Errorf("기존 팩을 덮으면 안 된다: %q", p1)
	}

	// 첫 팩이 그대로 남아 있어야 한다.
	got, err := pack.Load(p1)
	if err != nil || len(got) != 1 {
		t.Errorf("첫 팩이 온전해야 한다: %v %+v", err, got)
	}
}

func TestWritePackCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "packs")
	ps := []pack.Problem{{ID: "x1", Dir: pack.KoToJa, Prompt: "a", Reference: "b", Style: pack.StylePlain}}
	if _, err := WritePack(dir, spec(), ps, at()); err != nil {
		t.Errorf("없는 디렉터리를 만들어야 한다: %v", err)
	}
}
