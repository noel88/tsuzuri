// Command tsuzuri는 포메라 DM250에서 오프라인으로 도는
// 한↔일 양방향 번역 작문 드릴이다.
//
// 네트워크는 팩을 받을 때(5)와 첨삭을 받을 때(6)만 쓴다.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/config"
	"github.com/noel88/tsuzuri/internal/drill"
	"github.com/noel88/tsuzuri/internal/export"
	"github.com/noel88/tsuzuri/internal/gen"
	"github.com/noel88/tsuzuri/internal/llm"
	"github.com/noel88/tsuzuri/internal/pack"
	"github.com/noel88/tsuzuri/internal/store"
	tsync "github.com/noel88/tsuzuri/internal/sync"
	"github.com/noel88/tsuzuri/internal/ui"
)

func main() {
	if err := newApp().run(); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

type app struct {
	dataDir    string
	configPath string
	termW      int
	in         *bufio.Reader
	out        io.Writer
	analyzers  map[pack.Direction]*analyze.Analyzer
}

func newApp() *app {
	dataDir := os.Getenv("TSUZURI_DATA")
	if dataDir == "" {
		dataDir = "."
	}
	return &app{
		dataDir:    dataDir,
		configPath: filepath.Join(dataDir, "config.toml"),
		termW:      ui.TermWidth(),
		in:         bufio.NewReader(os.Stdin),
		out:        os.Stdout,
		analyzers:  map[pack.Direction]*analyze.Analyzer{},
	}
}

// packSet은 한 방향의 문제 묶음이다. 초기화면의 자료실 항목 하나에 대응한다.
type packSet struct {
	key      string
	dir      pack.Direction
	problems []pack.Problem
}

func (p packSet) label() string {
	levels, topics := map[string]bool{}, map[string]bool{}
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
		ks = append(ks[:2:2], "…")
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

func (a *app) run() error {
	for {
		sets, err := a.loadPackSets()
		if err != nil {
			return err
		}
		st, err := a.status(sets)
		if err != nil {
			return err
		}

		choices := make([]ui.Choice, 0, len(sets))
		for _, s := range sets {
			choices = append(choices, ui.Choice{
				Key:   s.key,
				Label: s.label(),
				Value: strconv.Itoa(len(s.problems)),
			})
		}
		fmt.Fprint(a.out, ui.RenderMenu(choices, st, a.termW))

		line, eof, err := ui.ReadLine(a.in)
		if err != nil {
			return err
		}
		key, ok := ui.ParseMenuKey(line)
		if !ok {
			return nil
		}

		quit, err := a.dispatch(key, sets, st)
		if err != nil {
			a.notice("오류: " + err.Error())
		}
		if quit || eof {
			return nil
		}
	}
}

// dispatch는 초기화면의 선택을 처리한다. 종료해야 하면 true를 돌려준다.
func (a *app) dispatch(key string, sets []packSet, st ui.Status) (bool, error) {
	switch key {
	case "2":
		return a.review()
	case "3":
		return false, a.export()
	case "4":
		return false, a.setup()
	case "5":
		return false, a.fetchPack()
	case "6":
		return false, a.fetchFeedback()
	}
	set, found := findSet(sets, key)
	if !found {
		a.notice(fmt.Sprintf("그런 번호가 없습니다: %q", key))
		return false, nil
	}
	return a.drill(set, st)
}

func (a *app) drill(set packSet, st ui.Status) (bool, error) {
	az, cached := a.analyzers[set.dir]
	if !cached {
		a.notice(fmt.Sprintf("사전을 읽는 중입니다 (%s)...", set.dir))
		var err error
		az, err = analyze.New(set.dir)
		if err != nil {
			return false, err
		}
		a.analyzers[set.dir] = az
	}

	s := &drill.Session{
		Problems: set.problems,
		Analyzer: az,
		DataDir:  a.dataDir,
		In:       a.in,
		Out:      a.out,
		TermW:    a.termW,
	}
	outcome, err := s.Run()
	if err != nil {
		return false, err
	}
	return outcome == drill.OutcomeQuit, nil
}

// review는 받은 첨삭을 순서대로 보여준다.
func (a *app) review() (bool, error) {
	feedback, err := store.ReadAll[tsync.Feedback](filepath.Join(a.dataDir, "feedback.jsonl"))
	if err != nil {
		return false, err
	}
	if len(feedback) == 0 {
		a.notice("받은 첨삭이 없습니다. 6번으로 첨삭을 받아 오세요.")
		return false, nil
	}

	attempts, err := store.ReadAll[store.Attempt](filepath.Join(a.dataDir, "attempts.jsonl"))
	if err != nil {
		return false, err
	}
	byID := make(map[string]store.Attempt, len(attempts))
	for _, at := range attempts {
		byID[at.ID] = at
	}
	problems, err := a.allProblems()
	if err != nil {
		return false, err
	}

	for i, f := range feedback {
		at, ok := byID[f.AttemptID]
		if !ok {
			continue
		}
		p := problems[at.PackID]
		st := ui.Status{Index: i + 1, Total: len(feedback)}
		fmt.Fprint(a.out, ui.RenderFeedback(p, at, f, st, a.termW))

		line, eof, err := ui.ReadLine(a.in)
		if err != nil {
			return false, err
		}
		switch ui.ParseCommand(line) {
		case ui.CmdQuit:
			return true, nil
		case ui.CmdMenu:
			return false, nil
		}
		if eof {
			return true, nil
		}
	}
	return false, nil
}

func (a *app) export() error {
	path, n, err := export.Write(a.dataDir, time.Now())
	if err != nil {
		return err
	}
	if n == 0 {
		a.notice("내보낼 첨삭이 없습니다.")
		return nil
	}
	a.notice(fmt.Sprintf("%d건을 내보냈습니다: %s\n  순정 포메라 모드에서 열어 읽을 수 있습니다.", n, path))
	return nil
}

func (a *app) setup() error {
	c, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	fmt.Fprint(a.out, ui.RenderSetup(c, ui.Status{}, a.termW))

	line, _, err := ui.ReadLine(a.in)
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(ui.SetupFields) {
		return nil // 메뉴로 조용히 돌아간다
	}
	field := ui.SetupFields[n-1]

	fmt.Fprintf(a.out, "\n  %s (그대로 두려면 Enter) >> ", field.Label)
	value, _, err := ui.ReadLine(a.in)
	if err != nil {
		return err
	}
	c, err = ui.ParseSetupAnswer(field.Key, value, c)
	if err != nil {
		return err
	}
	if err := config.Save(a.configPath, c); err != nil {
		return err
	}
	a.notice("저장했습니다.")
	return nil
}

// fetchPack은 새 문제 팩을 받아온다. 네트워크가 필요하다.
func (a *app) fetchPack() error {
	c, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	client, err := a.onlineClient(c)
	if err != nil {
		return err
	}

	dir := pack.KoToJa
	fmt.Fprint(a.out, "\n  방향 [1] 한국어→일본어  [2] 일본어→한국어 >> ")
	if line, _, err := ui.ReadLine(a.in); err == nil && strings.TrimSpace(line) == "2" {
		dir = pack.JaToKo
	}

	fmt.Fprintf(a.out, "  레벨 (기본 %s) >> ", c.Level)
	level, _, _ := ui.ReadLine(a.in)
	if strings.TrimSpace(level) == "" {
		level = c.Level
	}

	fmt.Fprintf(a.out, "  주제 (기본 %s) >> ", orNone(c.Topic))
	topic, _, _ := ui.ReadLine(a.in)
	if strings.TrimSpace(topic) == "" {
		topic = c.Topic
	}

	fmt.Fprintf(a.out, "  문항 수 (기본 %d) >> ", c.PackSize)
	sizeLine, _, _ := ui.ReadLine(a.in)
	count := c.PackSize
	if n, err := strconv.Atoi(strings.TrimSpace(sizeLine)); err == nil && n > 0 {
		count = n
	}

	spec := gen.Spec{Dir: dir, Level: strings.TrimSpace(level), Topic: strings.TrimSpace(topic), Count: count}
	a.notice(fmt.Sprintf("%d문항을 만드는 중입니다. 몇 분 걸릴 수 있습니다...", count))

	ps, err := gen.Generate(context.Background(), client, spec)
	if err != nil {
		return err
	}
	path, err := gen.WritePack(filepath.Join(a.dataDir, "packs"), spec, ps, time.Now())
	if err != nil {
		return err
	}
	a.notice(fmt.Sprintf("%d문항을 받았습니다: %s", len(ps), filepath.Base(path)))
	return nil
}

// fetchFeedback은 큐에 쌓인 답안의 첨삭을 받아온다. 네트워크가 필요하다.
func (a *app) fetchFeedback() error {
	c, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	client, err := a.onlineClient(c)
	if err != nil {
		return err
	}

	a.notice("첨삭을 받아오는 중입니다...")
	n, err := tsync.Run(context.Background(), client, a.dataDir, time.Now)
	if err != nil {
		return err
	}
	if n == 0 {
		a.notice("처리할 항목이 없습니다.")
		return nil
	}
	a.notice(fmt.Sprintf("%d건의 첨삭을 받았습니다. 2번에서 볼 수 있습니다.", n))
	return nil
}

// onlineClient는 사전 점검을 통과한 뒤에만 클라이언트를 만든다.
// 실기에서 처음 막힐 때 원인을 알려주는 것이 이 순서의 이유다.
func (a *app) onlineClient(c config.Config) (llm.Client, error) {
	if err := (llm.Preflight{}).Check(context.Background()); err != nil {
		return nil, err
	}
	return llm.NewAnthropic(c)
}

func (a *app) loadPackSets() ([]packSet, error) {
	var sets []packSet
	for i, d := range []pack.Direction{pack.KoToJa, pack.JaToKo} {
		ps, err := pack.LoadDir(filepath.Join(a.dataDir, "packs"), d)
		if err != nil {
			return nil, err
		}
		if len(ps) == 0 {
			continue
		}
		sets = append(sets, packSet{key: strconv.Itoa(41 + i), dir: d, problems: ps})
	}
	return sets, nil
}

func (a *app) allProblems() (map[string]pack.Problem, error) {
	out := map[string]pack.Problem{}
	for _, d := range []pack.Direction{pack.KoToJa, pack.JaToKo} {
		ps, err := pack.LoadDir(filepath.Join(a.dataDir, "packs"), d)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			out[p.ID] = p
		}
	}
	return out, nil
}

func (a *app) status(sets []packSet) (ui.Status, error) {
	queue, err := store.ReadAll[store.QueueItem](filepath.Join(a.dataDir, "queue.jsonl"))
	if err != nil {
		return ui.Status{}, err
	}
	feedback, err := store.ReadAll[tsync.Feedback](filepath.Join(a.dataDir, "feedback.jsonl"))
	if err != nil {
		return ui.Status{}, err
	}
	total := 0
	for _, s := range sets {
		total += len(s.problems)
	}
	return ui.Status{Total: total, QueueLen: len(queue), Feedback: len(feedback)}, nil
}

func (a *app) notice(msg string) {
	fmt.Fprintf(a.out, "\n  %s\n\n", msg)
}

func findSet(sets []packSet, key string) (packSet, bool) {
	for _, s := range sets {
		if s.key == key {
			return s, true
		}
	}
	return packSet{}, false
}

func orNone(s string) string {
	if s == "" {
		return "(없음)"
	}
	return s
}
