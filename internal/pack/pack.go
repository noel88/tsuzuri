package pack

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Direction은 문제의 출제 방향이다.
// 사용자가 작문하는 언어는 이것의 반대편이라는 점에 주의한다.
type Direction string

const (
	// KoToJa: 한국어 제시문 → 일본어로 작문
	KoToJa Direction = "ko2ja"
	// JaToKo: 일본어 제시문 → 한국어로 작문
	JaToKo Direction = "ja2ko"
)

// Style은 문제가 요구하는 문체다.
type Style string

const (
	StylePlain  Style = "plain"  // 보통체 (だ·である / 한다체)
	StylePolite Style = "polite" // 정중체 (です·ます / 해요체·합니다체)
)

// Problem은 문제 하나다.
//
// KeyPoints와 Traps는 팩 생성 시점에 LLM에게서 함께 받아둔 채점 근거다.
// 오프라인 분석이 이것에 의존하므로, 온라인 호출 한 번으로 수백 문제어치의
// 채점 지능을 미리 확보하는 구조다.
type Problem struct {
	ID        string    `json:"id"`
	Dir       Direction `json:"dir"`
	Level     string    `json:"level"`
	Topic     string    `json:"topic"`
	Prompt    string    `json:"prompt"`
	Reference string    `json:"reference"`
	KeyPoints []string  `json:"key_points"`
	Traps     []string  `json:"traps"`
	Style     Style     `json:"style"`
}

// Load는 JSON Lines 팩 파일 하나를 읽는다. 빈 줄은 건너뛴다.
func Load(path string) ([]Problem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Problem
	sc := bufio.NewScanner(f)
	// 기본 버퍼는 64KB다. 긴 문장이 들어갈 수 있으니 넉넉히 잡는다.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var p Problem
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			return nil, fmt.Errorf("%s %d번째 줄: %w", path, line, err)
		}
		out = append(out, p)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ByID는 디렉터리의 모든 팩을 읽어 ID로 찾을 수 있는 map을 만든다.
//
// 답안은 문제 ID만 기억하므로, ID가 겹치면 엉뚱한 문제로 채점하고
// 그 비용까지 청구된다. 겹치는 ID를 만나면 오류로 보고한다.
func ByID(dir string) (map[string]Problem, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	out := map[string]Problem{}
	from := map[string]string{}
	for _, path := range paths {
		ps, err := Load(path)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			if prev, dup := from[p.ID]; dup {
				return nil, fmt.Errorf(
					"문제 id %q가 %s와 %s에 겹칩니다. 한쪽 팩을 옮기거나 지우세요",
					p.ID, filepath.Base(prev), filepath.Base(path))
			}
			from[p.ID] = path
			out[p.ID] = p
		}
	}
	return out, nil
}

// LoadDir은 디렉터리의 모든 .jsonl을 읽어 방향이 일치하는 문제만 돌려준다.
// 한 세션은 한 방향만 다루므로 사전도 하나만 상주하게 된다.
func LoadDir(dir string, d Direction) ([]Problem, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []Problem
	for _, p := range paths {
		ps, err := Load(p)
		if err != nil {
			return nil, err
		}
		for _, pr := range ps {
			if pr.Dir == d {
				out = append(out, pr)
			}
		}
	}
	return out, nil
}
