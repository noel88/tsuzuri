// Command tsuzuri는 포메라 DM250에서 오프라인으로 도는
// 한↔일 양방향 번역 작문 드릴이다.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/drill"
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
	"github.com/noel88/tsuzuri/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

// packSet은 한 방향의 문제 묶음이다. 초기화면의 자료실 항목 하나에 대응한다.
type packSet struct {
	key      string
	dir      pack.Direction
	problems []pack.Problem
}

func (p packSet) label() string {
	levels := map[string]bool{}
	topics := map[string]bool{}
	for _, pr := range p.problems {
		if pr.Level != "" {
			levels[pr.Level] = true
		}
		if pr.Topic != "" {
			topics[pr.Topic] = true
		}
	}
	return fmt.Sprintf("%s  %s %s", p.dir, joinKeys(levels), joinKeys(topics))
}

func joinKeys(m map[string]bool) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	if len(ks) > 2 {
		ks = append(ks[:2], "…")
	}
	out := ""
	for i, k := range ks {
		if i > 0 {
			out += "·"
		}
		out += k
	}
	return out
}

func run() error {
	dataDir := os.Getenv("TSUZURI_DATA")
	if dataDir == "" {
		dataDir = "."
	}

	sets, err := loadPackSets(filepath.Join(dataDir, "packs"))
	if err != nil {
		return err
	}

	in := bufio.NewReader(os.Stdin)
	termW := ui.TermWidth()

	// 분석기는 방향마다 하나씩만 만든다. 사전 로딩이 비싸다.
	analyzers := map[pack.Direction]*analyze.Analyzer{}

	for {
		queue, err := store.ReadAll[store.QueueItem](filepath.Join(dataDir, "queue.jsonl"))
		if err != nil {
			return err
		}

		choices := make([]ui.Choice, 0, len(sets))
		total := 0
		for _, s := range sets {
			total += len(s.problems)
			choices = append(choices, ui.Choice{
				Key:   s.key,
				Label: s.label(),
				Value: fmt.Sprint(len(s.problems)),
			})
		}

		st := ui.Status{Total: total, QueueLen: len(queue)}
		fmt.Print(ui.RenderMenu(choices, st, termW))

		line, eof, err := ui.ReadLine(in)
		if err != nil {
			return err
		}
		key, ok := ui.ParseMenuKey(line)
		if !ok {
			return nil
		}
		if eof && line == "" {
			return nil
		}

		set, found := findSet(sets, key)
		if !found {
			fmt.Printf("\n  그런 번호가 없습니다: %q\n\n", line)
			if eof {
				return nil
			}
			continue
		}

		az, cached := analyzers[set.dir]
		if !cached {
			fmt.Printf("\n  사전을 읽는 중입니다 (%s)...\n", set.dir)
			az, err = analyze.New(set.dir)
			if err != nil {
				return err
			}
			analyzers[set.dir] = az
		}

		s := &drill.Session{
			Problems: set.problems,
			Analyzer: az,
			DataDir:  dataDir,
			In:       in,
			Out:      os.Stdout,
			TermW:    termW,
		}
		outcome, err := s.Run()
		if err != nil {
			return err
		}
		if outcome == drill.OutcomeQuit {
			return nil
		}
	}
}

// loadPackSets는 팩을 방향별로 묶는다. 한 세션은 한 방향만 다룬다.
func loadPackSets(dir string) ([]packSet, error) {
	var sets []packSet
	for i, d := range []pack.Direction{pack.KoToJa, pack.JaToKo} {
		ps, err := pack.LoadDir(dir, d)
		if err != nil {
			return nil, err
		}
		if len(ps) == 0 {
			continue
		}
		sets = append(sets, packSet{
			key:      fmt.Sprintf("%d", 41+i),
			dir:      d,
			problems: ps,
		})
	}
	return sets, nil
}

func findSet(sets []packSet, key string) (packSet, bool) {
	for _, s := range sets {
		if s.key == key {
			return s, true
		}
	}
	return packSet{}, false
}
