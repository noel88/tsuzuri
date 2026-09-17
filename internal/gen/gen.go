// Package gen은 문제 팩을 생성한다.
package gen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/noel88/tsuzuri/internal/llm"
	"github.com/noel88/tsuzuri/internal/pack"
)

// Spec은 어떤 팩을 만들지다.
type Spec struct {
	Dir   pack.Direction
	Level string
	Topic string
	Count int
}

const systemPrompt = `당신은 한국어 화자를 위한 일본어 작문 학습 교재를 만듭니다.

번역 작문 연습용 문장 쌍을 만드세요. 각 문항은 제시문과 모범 번역,
그리고 **오프라인 채점에 쓸 근거**로 이루어집니다.

지켜야 할 것:

- 제시문은 학습자가 실제로 쓸 법한 자연스러운 문장이어야 합니다.
  교과서 예문 같은 인공적인 문장은 피하세요.
- **길이를 섞으세요.** 비슷한 길이만 나오면 같은 종류의 연습만 반복됩니다.
  아래 셋을 대략 같은 수로 내세요.
    단문 — 절 하나. 짧게 끊어 말하는 문장. (예: "비가 올 것 같아서 우산을
      가져왔습니다.")
    중문 — 절 두 개가 이어진 문장. 이유·조건·역접 같은 연결이 하나 있습니다.
      (예: "어제 처음 간 카페가 생각보다 조용해서 오래 앉아 있었다.")
    장문 — 두 문장이거나, 절이 셋 이상 얽힌 문장. 앞뒤 문맥이 이어져야
      합니다. (예: "요즘 계속 바빠서 방 청소를 할 기력도 없어. 방이 점점 더
      지저분해지고 있어.")
  길이는 문항 수를 채우려고 늘리는 것이 아닙니다. 긴 문항은 긴 만큼
  연결·시제·문맥을 다루게 하세요.
- reference는 **유일한 정답이 아니라 모범 예시 하나**입니다. 학습자가
  다른 표현으로 같은 뜻을 쓸 수 있다는 전제로 작성하세요.
- key_points는 **reference 안에 글자 그대로 들어 있는 짧은 표현** 2~4개입니다.
  채점기가 학습자의 답안에서 이 문자열을 찾아 확인하므로, 설명문을 쓰면
  절대 찾지 못합니다.
    좋음: "思ったより", "〜ていた", "ことにする", "気力"
    나쁨: "원인·이유의 て형 접속(忙しくて)"   ← 설명문
    나쁨: "자동사 壊れる와 타동사 壊す 구별"   ← 설명문
    나쁨: "〜みたいだ / 〜ようだ"              ← 여러 개를 한 항목에
  괄호, 슬래시, 쉼표, 한국어 설명을 넣지 마세요. 활용형은 사전형 대신
  reference에 나온 그대로 쓰세요. 조사 하나처럼 너무 작은 것이나 문장
  전체처럼 너무 큰 것은 피하세요.
- traps에는 설명을 자유롭게 쓰세요. 거기는 사람이 읽는 자리입니다.
- traps는 한국어 화자가 이 문장에서 실제로 저지르는 오류입니다.
  없으면 빈 배열로 두세요.
- style은 문제가 요구하는 문체입니다. plain(보통체) 또는 polite(정중체).
- id는 문항마다 다른 짧은 문자열입니다.`

var problemSchema = map[string]any{
	"problems": map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":        map[string]any{"type": "string"},
				"dir":       map[string]any{"type": "string", "enum": []string{"ko2ja", "ja2ko"}},
				"level":     map[string]any{"type": "string"},
				"topic":     map[string]any{"type": "string"},
				"prompt":    map[string]any{"type": "string"},
				"reference": map[string]any{"type": "string"},
				"key_points": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string", "maxLength": 20},
				},
				"traps": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"style": map[string]any{"type": "string", "enum": []string{"plain", "polite"}},
			},
			"required":             []string{"id", "dir", "level", "topic", "prompt", "reference", "key_points", "traps", "style"},
			"additionalProperties": false,
		},
	},
}

// lengthMix는 문항 수를 단문·중문·장문으로 나눈다.
//
// 배분을 숫자로 적어 주지 않으면 모델이 비슷한 길이만 낸다. 「섞어 주세요」
// 는 지켜지는 편이 아니고, 몇 개씩인지 적으면 지켜진다.
//
// 나머지는 중문에 먼저 준다. 셋 중 가장 쓸모가 많은 길이여서다 — 절 두 개를
// 잇는 연습이 번역 작문에서 제일 자주 걸린다.
func lengthMix(count int) (short, mid, long int) {
	short, mid, long = count/3, count/3, count/3
	switch count % 3 {
	case 1:
		mid++
	case 2:
		mid++
		short++
	}
	return short, mid, long
}

// LengthOf는 제시문이 어느 길이인지 본다. 받은 팩의 배분을 알리는 데 쓴다.
//
// 글자 수로 가른다. 절을 세려면 형태소 분석이 필요한데, 팩을 받는 시점에는
// 사전이 올라와 있지 않을 수 있고 그것을 위해 사전을 읽는 것은 수십 초다.
func LengthOf(prompt string) string {
	switch n := len([]rune(prompt)); {
	case n <= 20:
		return "단문"
	case n <= 40:
		return "중문"
	default:
		return "장문"
	}
}

// Generate는 문제 팩을 만든다.
func Generate(ctx context.Context, c llm.Client, s Spec) ([]pack.Problem, error) {
	if s.Count <= 0 {
		return nil, fmt.Errorf("문항 수가 0 이하입니다: %d", s.Count)
	}
	short, mid, long := lengthMix(s.Count)
	user := fmt.Sprintf(
		"방향: %s (%s)\n레벨: %s\n주제: %s\n문항 수: %d\n"+
			"길이 배분: 단문 %d개, 중문 %d개, 장문 %d개\n\n"+
			"위 조건으로 문장 쌍을 만들어 emit_problems 도구로 제출하세요.",
		s.Dir, directionLabel(s.Dir), s.Level, s.Topic, s.Count, short, mid, long)

	raw, err := c.Complete(ctx, llm.Request{
		System:    systemPrompt,
		User:      user,
		ToolName:  "emit_problems",
		ToolDesc:  "생성한 문장 쌍 목록을 제출합니다.",
		Schema:    problemSchema,
		Required:  []string{"problems"},
		MaxTokens: 32000,
	})
	if err != nil {
		return nil, err
	}

	var out struct {
		Problems []pack.Problem `json:"problems"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("응답을 해석하지 못했습니다: %w", err)
	}
	if len(out.Problems) == 0 {
		return nil, fmt.Errorf("문항이 하나도 오지 않았습니다")
	}
	// 문항 하나가 불량이라고 팩 전체를 버리지 않는다. 이미 값을 치른
	// 생성 결과이므로, 쓸 수 있는 것은 살리고 몇 개를 버렸는지 알린다.
	seen := map[string]bool{}
	kept := make([]pack.Problem, 0, len(out.Problems))
	var bad []string
	for i, p := range out.Problems {
		if err := validate(p, s); err != nil {
			bad = append(bad, fmt.Sprintf("%d번째(%v)", i+1, err))
			continue
		}
		p, dropped := pruneKeyPoints(p)
		if len(dropped) > 0 {
			bad = append(bad, fmt.Sprintf("%d번째(모범답안에 없는 핵심 표현 %d개 제거: %s)",
				i+1, len(dropped), strings.Join(dropped, " / ")))
		}
		if seen[p.ID] {
			bad = append(bad, fmt.Sprintf("%d번째(id %q 중복)", i+1, p.ID))
			continue
		}
		seen[p.ID] = true
		kept = append(kept, p)
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("쓸 수 있는 문항이 없습니다: %s", strings.Join(bad, ", "))
	}
	return kept, nil
}

func directionLabel(d pack.Direction) string {
	if d == pack.KoToJa {
		return "한국어 제시문 → 일본어로 작문"
	}
	return "일본어 제시문 → 한국어로 작문"
}

// pruneKeyPoints는 모범답안에 실제로 없는 핵심 표현을 걸러낸다.
//
// 모델이 "원인·이유의 て형 접속(忙しくて)"처럼 설명문을 넣으면 채점기가
// 답안에서 절대 찾지 못해, 모범답안을 그대로 써도 전부 "빠짐"으로 뜬다.
// 프롬프트로 막되, 그것만 믿지 않는다.
func pruneKeyPoints(p pack.Problem) (pack.Problem, []string) {
	ref := squeeze(stripTilde(p.Reference))
	kept := make([]string, 0, len(p.KeyPoints))
	var dropped []string
	for _, kp := range p.KeyPoints {
		needle := squeeze(stripTilde(kp))
		if needle != "" && strings.Contains(ref, needle) {
			kept = append(kept, kp)
			continue
		}
		dropped = append(dropped, kp)
	}
	p.KeyPoints = kept
	return p, dropped
}

// stripTilde와 squeeze는 analyze.Coverage가 쓰는 정규화와 같아야 한다.
// 거기서 찾을 수 없는 표현은 여기서도 걸러야 하기 때문이다.
func stripTilde(s string) string {
	for _, t := range []string{"〜", "～", "~"} {
		s = strings.ReplaceAll(s, t, "")
	}
	return strings.TrimSpace(s)
}

func squeeze(s string) string {
	return strings.Join(strings.Fields(s), "")
}

func validate(p pack.Problem, s Spec) error {
	if p.Dir != s.Dir {
		return fmt.Errorf("방향이 %q인데 %q를 요청했습니다", p.Dir, s.Dir)
	}
	if p.Prompt == "" {
		return fmt.Errorf("제시문이 비어 있습니다")
	}
	if p.Reference == "" {
		return fmt.Errorf("참조 번역이 비어 있습니다")
	}
	if p.Style != pack.StylePlain && p.Style != pack.StylePolite {
		return fmt.Errorf("알 수 없는 문체: %q", p.Style)
	}
	if p.ID == "" {
		return fmt.Errorf("id가 비어 있습니다")
	}
	return nil
}

// safeName은 파일 이름에 쓸 수 없는 글자를 바꾼다.
func safeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "pack"
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			b.WriteRune('-')
		default:
			if r < 0x20 {
				b.WriteRune('-')
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// WritePack은 팩을 새 파일로 쓴다. 기존 팩을 절대 덮지 않는다.
func WritePack(dir string, s Spec, ps []pack.Problem, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// 레벨·주제는 사용자가 자유롭게 적는다. 파일 이름에 쓸 수 없는 글자가
	// 들어가면 방금 값을 치른 팩을 저장하지 못하고 잃는다.
	base := fmt.Sprintf("%s-%s-%s", s.Dir, safeName(s.Level), now.Format("20060102-150405"))
	path := filepath.Join(dir, base+".jsonl")
	// 이름이 겹치면 번호를 붙인다. NotExist가 아닌 오류(권한, SD카드 I/O)는
	// 이름을 바꿔도 사라지지 않으므로 루프를 빠져나가 O_EXCL이 판정하게 한다.
	for n := 2; n < 100; n++ {
		_, err := os.Stat(path)
		if err != nil {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.jsonl", base, n))
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// ID를 팩 이름으로 네임스페이스한다.
	//
	// 모델은 매번 p001부터 번호를 매기므로 팩끼리 ID가 겹친다. 답안은
	// 문제 ID만 기억하기 때문에, 겹치면 엉뚱한 문제로 채점하고 그 비용을
	// 청구받는다. 파일명은 시각까지 포함하므로 팩마다 다르다.
	prefix := strings.TrimSuffix(filepath.Base(path), ".jsonl") + "/"
	enc := json.NewEncoder(f)
	for _, p := range ps {
		p.ID = prefix + p.ID
		if err := enc.Encode(p); err != nil {
			return "", err
		}
	}
	return path, f.Sync()
}
