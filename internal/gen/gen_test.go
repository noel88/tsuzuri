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
	got, _, err := Generate(context.Background(), f, spec())
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
	if _, _, err := Generate(context.Background(), f, spec()); err != nil {
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
	if _, _, err := Generate(context.Background(), f, spec()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Got.System, "유일한 정답") {
		t.Errorf("참조가 유일한 정답이 아님을 명시해야 한다:\n%s", f.Got.System)
	}
}

func TestGenerateRejectsWrongDirection(t *testing.T) {
	f := &llm.FakeClient{Reply: strings.ReplaceAll(goodReply, `"ko2ja"`, `"ja2ko"`)}
	if _, _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("요청한 방향과 다르면 거부해야 한다")
	}
}

func TestGenerateRejectsEmptyReference(t *testing.T) {
	f := &llm.FakeClient{Reply: `{"problems":[{"id":"x1","dir":"ko2ja","prompt":"질문","reference":"","style":"plain"}]}`}
	if _, _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("참조가 비면 드릴이 성립하지 않는다")
	}
}

// 문항 하나가 불량이라고 팩 전체를 버리면, 이미 값을 치른 생성 결과를
// 통째로 잃는다. 쓸 수 있는 것은 살린다.
func TestGenerateDropsBadItemsAndKeepsTheRest(t *testing.T) {
	// 둘째 문항의 id가 첫째와 겹친다.
	f := &llm.FakeClient{Reply: strings.ReplaceAll(goodReply, `"x2"`, `"x1"`)}
	got, _, err := Generate(context.Background(), f, spec())
	if err != nil {
		t.Fatalf("쓸 수 있는 문항이 있으면 실패하면 안 된다: %v", err)
	}
	if len(got) != 1 || got[0].ID != "x1" {
		t.Errorf("겹치는 문항만 버려야 한다: %+v", got)
	}
}

func TestGenerateFailsOnlyWhenNothingIsUsable(t *testing.T) {
	f := &llm.FakeClient{Reply: `{"problems":[{"id":"x1","dir":"ko2ja","prompt":"질문","reference":"","style":"plain"}]}`}
	if _, _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("쓸 수 있는 문항이 하나도 없으면 오류여야 한다")
	}
}

func TestGenerateRejectsMalformedJSON(t *testing.T) {
	f := &llm.FakeClient{Reply: `이건 JSON이 아니다`}
	if _, _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("깨진 응답은 오류여야 한다")
	}
}

func TestGenerateRejectsEmptyResult(t *testing.T) {
	f := &llm.FakeClient{Reply: `{"problems":[]}`}
	if _, _, err := Generate(context.Background(), f, spec()); err == nil {
		t.Error("빈 결과는 오류여야 한다")
	}
}

func TestGenerateRejectsZeroCount(t *testing.T) {
	f := &llm.FakeClient{Reply: goodReply}
	if _, _, err := Generate(context.Background(), f, Spec{Dir: pack.KoToJa, Count: 0}); err == nil {
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
	if len(got) != 1 {
		t.Fatalf("왕복 실패: %+v", got)
	}
	// WritePack이 ID를 팩 이름으로 네임스페이스한다.
	// 모델은 팩마다 p001부터 번호를 매기므로 그대로 두면 팩끼리 겹친다.
	if !strings.HasSuffix(got[0].ID, "/x1") {
		t.Errorf("ID에 팩 이름이 붙어야 한다: %q", got[0].ID)
	}
	if got[0].ID == "x1" {
		t.Error("네임스페이스가 적용되지 않았다")
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

func TestWritePackNamespacesIDsPerPack(t *testing.T) {
	// 두 팩이 같은 ID 집합을 내놓아도 충돌하지 않아야 한다.
	dir := t.TempDir()
	ps := []pack.Problem{{ID: "p001", Dir: pack.KoToJa, Prompt: "a", Reference: "b", Style: pack.StylePlain}}

	if _, err := WritePack(dir, spec(), ps, at()); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePack(dir, spec(), ps, at().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	byID, _, err := pack.ByID(dir)
	if err != nil {
		t.Fatalf("두 팩의 ID가 겹치면 안 된다: %v", err)
	}
	if len(byID) != 2 {
		t.Errorf("문제 개수 = %d, 기대 2", len(byID))
	}
}

func TestWritePackCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "packs")
	ps := []pack.Problem{{ID: "x1", Dir: pack.KoToJa, Prompt: "a", Reference: "b", Style: pack.StylePlain}}
	if _, err := WritePack(dir, spec(), ps, at()); err != nil {
		t.Errorf("없는 디렉터리를 만들어야 한다: %v", err)
	}
}

// 레벨·주제는 사용자가 자유롭게 적는다. 파일 이름에 쓸 수 없는 글자가
// 들어가면 방금 값을 치른 팩을 저장하지 못하고 잃는다.
func TestWritePackSurvivesAwkwardLevelNames(t *testing.T) {
	dir := t.TempDir()
	ps := []pack.Problem{{ID: "x1", Dir: pack.KoToJa, Prompt: "a", Reference: "b", Style: pack.StylePlain}}
	for _, level := range []string{"N3/N4", "일상: 카페", "  ", "a*b?c"} {
		s := Spec{Dir: pack.KoToJa, Level: level, Topic: "일상", Count: 1}
		path, err := WritePack(dir, s, ps, at())
		if err != nil {
			t.Errorf("레벨 %q에서 저장 실패: %v", level, err)
			continue
		}
		if _, err := pack.Load(path); err != nil {
			t.Errorf("레벨 %q로 만든 팩을 읽지 못한다: %v", level, err)
		}
	}
}

// 모델이 "원인·이유의 て형 접속(忙しくて)"처럼 설명문을 넣으면 채점기가
// 답안에서 찾지 못해, 모범답안을 그대로 써도 전부 "빠짐"으로 뜬다.
// 실제 API 응답에서 이 일이 일어났다.
func TestGenerateDropsKeyPointsNotInReference(t *testing.T) {
	reply := `{"problems":[{"id":"x1","dir":"ko2ja","level":"N3","topic":"일상",
	  "prompt":"요즘 일이 바빠서 기력이 없어요.",
	  "reference":"最近仕事が忙しくて、気力がありません。",
	  "key_points":["忙しくて","원인·이유의 て형 접속(忙しくて)","気力","何も+부정 호응"],
	  "traps":[],"style":"polite"}]}`
	f := &llm.FakeClient{Reply: reply}

	got, _, err := Generate(context.Background(), f, spec())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("문항 = %d", len(got))
	}
	want := []string{"忙しくて", "気力"}
	if len(got[0].KeyPoints) != len(want) {
		t.Fatalf("핵심 표현 = %v, 기대 %v (설명문은 걸러야 한다)", got[0].KeyPoints, want)
	}
	for i := range want {
		if got[0].KeyPoints[i] != want[i] {
			t.Errorf("핵심 표현 = %v, 기대 %v", got[0].KeyPoints, want)
			break
		}
	}
}

func TestGeneratePromptDemandsLiteralKeyPoints(t *testing.T) {
	f := &llm.FakeClient{Reply: goodReply}
	if _, _, err := Generate(context.Background(), f, spec()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Got.System, "글자 그대로") {
		t.Errorf("핵심 표현이 reference에 그대로 있어야 함을 명시해야 한다:\n%s", f.Got.System)
	}
}

func TestLengthMixSplitsEvenly(t *testing.T) {
	for _, count := range []int{1, 2, 3, 10, 20, 25, 50} {
		s, m, l := lengthMix(count)
		if s+m+l != count {
			t.Errorf("%d문항 = %d+%d+%d, 합이 안 맞는다", count, s, m, l)
		}
		if s < 0 || m < 0 || l < 0 {
			t.Errorf("%d문항에서 음수가 나왔다: %d %d %d", count, s, m, l)
		}
		// 어느 하나가 나머지보다 둘 이상 많으면 섞인 것이 아니다.
		max, min := s, s
		for _, v := range []int{m, l} {
			if v > max {
				max = v
			}
			if v < min {
				min = v
			}
		}
		if max-min > 1 {
			t.Errorf("%d문항 배분이 치우쳤다: 단문 %d 중문 %d 장문 %d", count, s, m, l)
		}
	}
}

func TestGenerateRefusesTooManyProblems(t *testing.T) {
	// 넘겨서 부르면 몇 분을 기다린 끝에 끊기고, 그동안 쓴 토큰은 그대로
	// 청구된다. 실기에서 300문항을 요청했다가 그렇게 잃었다.
	f := &llm.FakeClient{Reply: `{"problems":[]}`}
	s := spec()
	s.Count = 300

	_, _, err := Generate(context.Background(), f, s)
	if err == nil {
		t.Fatal("한도를 넘긴 요청을 그대로 보냈다")
	}
	if f.Calls != 0 {
		t.Errorf("API를 %d번 불렀다 — 부르기 전에 막아야 한다", f.Calls)
	}
	if !strings.Contains(err.Error(), "나눠 받으세요") {
		t.Errorf("어떻게 하라는 말이 없다: %v", err)
	}
}
