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
- reference는 **유일한 정답이 아니라 모범 예시 하나**입니다. 학습자가
  다른 표현으로 같은 뜻을 쓸 수 있다는 전제로 작성하세요.
- key_points는 그 레벨에서 연습 가치가 있는 표현 2~4개입니다.
  조사 하나처럼 너무 작은 것이나, 문장 전체처럼 너무 큰 것은 피하세요.
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
				"id":         map[string]any{"type": "string"},
				"dir":        map[string]any{"type": "string", "enum": []string{"ko2ja", "ja2ko"}},
				"level":      map[string]any{"type": "string"},
				"topic":      map[string]any{"type": "string"},
				"prompt":     map[string]any{"type": "string"},
				"reference":  map[string]any{"type": "string"},
				"key_points": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"traps":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"style":      map[string]any{"type": "string", "enum": []string{"plain", "polite"}},
			},
			"required":             []string{"id", "dir", "level", "topic", "prompt", "reference", "key_points", "traps", "style"},
			"additionalProperties": false,
		},
	},
}

// Generate는 문제 팩을 만든다.
func Generate(ctx context.Context, c llm.Client, s Spec) ([]pack.Problem, error) {
	if s.Count <= 0 {
		return nil, fmt.Errorf("문항 수가 0 이하입니다: %d", s.Count)
	}
	user := fmt.Sprintf(
		"방향: %s (%s)\n레벨: %s\n주제: %s\n문항 수: %d\n\n"+
			"위 조건으로 문장 쌍을 만들어 emit_problems 도구로 제출하세요.",
		s.Dir, directionLabel(s.Dir), s.Level, s.Topic, s.Count)

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
	seen := map[string]bool{}
	for i, p := range out.Problems {
		if err := validate(p, s); err != nil {
			return nil, fmt.Errorf("%d번째 문항: %w", i+1, err)
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("%d번째 문항: id %q가 중복입니다", i+1, p.ID)
		}
		seen[p.ID] = true
	}
	return out.Problems, nil
}

func directionLabel(d pack.Direction) string {
	if d == pack.KoToJa {
		return "한국어 제시문 → 일본어로 작문"
	}
	return "일본어 제시문 → 한국어로 작문"
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

// WritePack은 팩을 새 파일로 쓴다. 기존 팩을 절대 덮지 않는다.
func WritePack(dir string, s Spec, ps []pack.Problem, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := fmt.Sprintf("%s-%s-%s", s.Dir, s.Level, now.Format("20060102-150405"))
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
