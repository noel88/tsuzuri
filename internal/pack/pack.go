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
	// Source는 이 문제가 들어 있던 팩 파일이다. 파일에는 없고 읽을 때 채운다.
	// 자료실이 팩 단위로 보이려면 문제가 어느 팩에서 왔는지 알아야 한다.
	Source string `json:"-"`

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

// NormalizeLevel은 "2"처럼 N을 뺀 레벨을 "N2"로 고친다.
//
// 레벨은 자유 문자열이라 무엇이든 받지만, 그 값이 그대로 생성 프롬프트에
// 들어가고 자료실에도 그대로 보인다. "2" 하나만 적힌 채로 팩을 받으면
// 엉뚱한 난이도가 나오는데, 알아차릴 때는 이미 과금된 뒤다. 실기에서
// level = "2" 로 50문항을 받은 적이 있다.
//
// 읽을 때도 고친다. 이미 "2" 로 받아 둔 팩이 자료실에서 "2" 로 보이면
// 사용자는 자기가 N2를 고른 것이 무시됐다고 읽는다.
func NormalizeLevel(s string) string {
	if len(s) == 1 && s[0] >= '1' && s[0] <= '5' {
		return "N" + s
	}
	return s
}

// Skip은 읽지 못해 건너뛴 팩 파일이다.
//
// 팩 하나가 깨졌다고 앱이 시작조차 못 하면 안 된다. 건너뛰되 무엇을 왜
// 건너뛰었는지 알려서, 사용자가 그 팩을 고치거나 지울 수 있게 한다.
type Skip struct {
	Path   string
	Reason string
}

// isSidecar는 팩이 아닌데 팩 디렉터리에 생기는 파일인지 본다.
//
// macOS는 FAT 카드에 파일을 쓸 때 확장 속성을 담은 AppleDouble 파일
// `._<이름>`을 같이 만든다. deploy/README가 권하는 방식(맥에서 SD에 복사)을
// 그대로 따르면 반드시 생긴다.
func isSidecar(name string) bool {
	return strings.HasPrefix(name, "._") || name == ".DS_Store"
}

// trimBOM은 손으로 만든 파일에 붙은 UTF-8 BOM을 떼어낸다.
func trimBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
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
		text := strings.TrimSpace(trimBOM(sc.Text()))
		if text == "" {
			continue
		}
		var p Problem
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			return nil, fmt.Errorf("%s %d번째 줄: %w", path, line, err)
		}
		p.Source = path
		p.Level = NormalizeLevel(p.Level)
		out = append(out, p)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// firstDuplicate는 이미 쓰인 id가 있는지 본다.
func firstDuplicate(ps []Problem, from map[string]string) (string, bool) {
	for _, p := range ps {
		if prev, dup := from[p.ID]; dup {
			return prev, true
		}
	}
	return "", false
}

// ByID는 디렉터리의 모든 팩을 읽어 ID로 찾을 수 있는 map을 만든다.
//
// 답안은 문제 ID만 기억하므로, ID가 겹치면 엉뚱한 문제로 채점하고
// 그 비용까지 청구된다. 겹치는 팩은 통째로 건너뛰고 무엇을 건너뛰었는지
// 알린다. LoadDir도 같은 팩을 건너뛰므로 드릴에도 나오지 않는다.
func ByID(dir string) (map[string]Problem, []Skip, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(paths)

	out := map[string]Problem{}
	from := map[string]string{}
	var skipped []Skip
	for _, path := range paths {
		if isSidecar(filepath.Base(path)) {
			continue
		}
		ps, err := Load(path)
		if err != nil {
			skipped = append(skipped, Skip{Path: path, Reason: err.Error()})
			continue
		}
		if prev, dup := firstDuplicate(ps, from); dup {
			skipped = append(skipped, Skip{
				Path:   path,
				Reason: fmt.Sprintf("문제 id가 %s와 겹칩니다", filepath.Base(prev)),
			})
			continue
		}
		for _, p := range ps {
			from[p.ID] = path
			out[p.ID] = p
		}
	}
	return out, skipped, nil
}

// LoadDir은 디렉터리의 모든 .jsonl을 읽어 방향이 일치하는 문제만 돌려준다.
// 한 세션은 한 방향만 다루므로 사전도 하나만 상주하게 된다.
func LoadDir(dir string, d Direction) ([]Problem, []Skip, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(paths)

	var out []Problem
	var skipped []Skip
	from := map[string]string{}
	for _, p := range paths {
		if isSidecar(filepath.Base(p)) {
			continue
		}
		ps, err := Load(p)
		if err != nil {
			// 팩 하나가 깨져도 나머지로 계속 쓸 수 있어야 한다.
			skipped = append(skipped, Skip{Path: p, Reason: err.Error()})
			continue
		}
		// id가 겹치는 팩은 통째로 건너뛴다.
		//
		// 드릴에서만 허용하고 나중에 막으면, 사용자가 겹친 팩을 지운 뒤
		// 남은 팩의 문제로 채점되어 엉뚱한 첨삭에 값을 치르게 된다.
		if prev, dup := firstDuplicate(ps, from); dup {
			skipped = append(skipped, Skip{
				Path:   p,
				Reason: fmt.Sprintf("문제 id가 %s와 겹칩니다", filepath.Base(prev)),
			})
			continue
		}
		for _, pr := range ps {
			from[pr.ID] = p
			if pr.Dir == d {
				out = append(out, pr)
			}
		}
	}
	return out, skipped, nil
}
