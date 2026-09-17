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
	"syscall"
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

	// 마지막 온라인 작업이 성공했는지. 초기화면 상태 표시에 쓴다.
	online bool

	// 이미 알린 팩 경로. 같은 경고를 메뉴마다 반복하지 않는다.
	warned map[string]bool

	// 사전은 한 번에 하나만 상주시킨다.
	//
	// 측정: 일본어(IPADIC) 88MB + 한국어(ko-dic) 211MB = 300MB 라이브 힙.
	// DM250은 RAM이 1GB이고 Go GC는 보통 라이브 힙의 2배쯤 RSS를 쓰므로,
	// 두 방향을 동시에 들고 있으면 OOM이다.
	analyzer    *analyze.Analyzer
	analyzerDir pack.Direction
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
		warned:     map[string]bool{},
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
	// 방향 전환으로 다시 실행된 경우, 곧바로 그 팩을 연다.
	pending := os.Getenv(gotoEnv)
	os.Unsetenv(gotoEnv)

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
		key, eof := pending, false
		if pending == "" {
			fmt.Fprint(a.out, ui.RenderMenu(choices, st, a.termW))

			line, isEOF, err := ui.ReadLine(a.in)
			if err != nil {
				return err
			}
			eof = isEOF
			var ok bool
			if key, ok = ui.ParseMenuKey(line); !ok {
				return nil
			}
		}
		pending = ""

		quit, err := a.dispatch(key, sets)
		if err != nil {
			a.notice("오류: " + err.Error())
		}
		if quit || eof {
			return nil
		}
	}
}

// dispatch는 초기화면의 선택을 처리한다. 종료해야 하면 true를 돌려준다.
func (a *app) dispatch(key string, sets []packSet) (bool, error) {
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
	return a.drill(set)
}

func (a *app) drill(set packSet) (bool, error) {
	az, err := a.analyzerFor(set.dir)
	if err != nil {
		return false, err
	}

	s := &drill.Session{
		Problems: set.problems,
		Analyzer: az,
		DataDir:  a.dataDir,
		In:       a.in,
		Out:      a.out,
		TermW:    a.termW,
		Online:   a.online,
	}
	outcome, err := s.Run()
	if err != nil {
		return false, err
	}
	return outcome == drill.OutcomeQuit, nil
}

// analyzerFor는 그 방향의 분석기를 돌려준다.
//
// 한 프로세스는 사전을 하나만 연다. 방향이 바뀌면 자기 자신을 다시
// 실행해서 OS가 이전 사전의 메모리를 통째로 회수하게 한다.
//
// 참조를 끊고 GC를 돌리는 방식은 통하지 않는다. kagome의 사전 패키지는
// 사전을 패키지 전역 변수에 sync.Once로 담아 두므로, 한 번 Dict()를
// 부르면 프로세스가 끝날 때까지 상주한다. 측정: 일본어 88MB,
// 한국어 211MB, 둘 다 열면 300MB — RAM 1GB 기기에서 감당할 수 없다.
//
// DictShrink()는 102MB까지 줄여 주지만 BaseForm과 Expression을 함께
// 버려서 분석이 성립하지 않는다 (internal/analyze/shrink_test.go).
func (a *app) analyzerFor(d pack.Direction) (*analyze.Analyzer, error) {
	if a.analyzer != nil {
		if a.analyzerDir == d {
			return a.analyzer, nil
		}
		return nil, a.restartFor(d)
	}
	az, err := a.loadDictionary(d)
	if err != nil {
		return nil, err
	}
	a.analyzer, a.analyzerDir = az, d
	return az, nil
}

// dictLoadEstimate는 포메라 DM250(Cortex-A7 816MHz)에서 잰 사전 로딩 시간이다.
//
// 맥에서는 1초도 걸리지 않지만 실기에서는 수십 초가 걸린다. 그 사이 화면이
// 멈춘 것처럼 보이면 고장으로 오해하므로, 얼마나 걸릴지 미리 알리고 진행을
// 보여준다. 기다리는 시간 자체는 줄일 수 없다 — kagome가 사전 구조를 만드는
// 비용이고, CPU는 816MHz가 상한이다.
var dictLoadEstimate = map[pack.Direction]time.Duration{
	pack.KoToJa: 17 * time.Second, // 일본어 사전(IPADIC)
	pack.JaToKo: 39 * time.Second, // 한국어 사전(mecab-ko-dic)
}

// loadDictionary는 사전을 읽으면서 진행을 보여준다.
func (a *app) loadDictionary(d pack.Direction) (*analyze.Analyzer, error) {
	est := dictLoadEstimate[d]
	if est > 0 {
		fmt.Fprintf(a.out, "\n  사전을 읽는 중입니다 (%s, 약 %d초 걸립니다)\n  ",
			d, int(est.Seconds()))
	} else {
		fmt.Fprintf(a.out, "\n  사전을 읽는 중입니다 (%s)\n  ", d)
	}

	// 진행 표시는 점 하나씩. 화면을 지우거나 커서를 옮기지 않는다 —
	// fbterm의 ANSI 지원 범위를 믿지 않기로 했기 때문이다(스펙 §6.1).
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				fmt.Fprint(a.out, ".")
			}
		}
	}()

	start := time.Now()
	az, err := analyze.New(d)
	close(stop)
	<-done // 점 찍기가 끝난 뒤에 이어 쓴다

	if err != nil {
		fmt.Fprintln(a.out)
		return nil, err
	}
	fmt.Fprintf(a.out, " 완료 (%.0f초)\n\n", time.Since(start).Seconds())
	return az, nil
}

// gotoEnv는 재실행 후 곧바로 열 메뉴 번호를 넘기는 환경변수다.
const gotoEnv = "TSUZURI_GOTO"

// restartFor는 다른 방향을 열기 위해 자기 자신을 다시 실행한다.
//
// 성공하면 돌아오지 않는다. exec가 프로세스 이미지를 교체하므로
// 이전 사전이 차지하던 메모리는 OS가 회수한다.
func (a *app) restartFor(d pack.Direction) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("방향을 바꾸려면 앱을 다시 실행해야 합니다: %w", err)
	}
	a.notice(fmt.Sprintf("방향이 바뀌어 다시 시작합니다 (%s → %s)...", a.analyzerDir, d))

	env := os.Environ()
	if key, ok := a.keyForDir(d); ok {
		env = append(env, gotoEnv+"="+key)
	}
	if err := syscall.Exec(self, os.Args, env); err != nil {
		return fmt.Errorf("다시 실행하지 못했습니다. 앱을 끄고 다시 켜 주세요: %w", err)
	}
	return nil // 도달하지 않는다
}

// keyForDir은 그 방향의 팩에 해당하는 메뉴 번호를 찾는다.
func (a *app) keyForDir(d pack.Direction) (string, bool) {
	sets, err := a.loadPackSets()
	if err != nil {
		return "", false
	}
	for _, s := range sets {
		if s.dir == d {
			return s.key, true
		}
	}
	return "", false
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
	problems, skipped, err := pack.ByID(filepath.Join(a.dataDir, "packs"))
	if err != nil {
		return false, err
	}
	a.reportSkipped(skipped)

	for i, f := range feedback {
		at, ok := byID[f.AttemptID]
		if !ok {
			continue
		}
		p, ok := problems[at.PackID]
		if !ok {
			// 팩이 사라졌다. 제시문 없이 첨삭만 보여주면 맥락이 없다.
			continue
		}
		st := ui.Status{Index: i + 1, Total: len(feedback), Online: a.online}
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
	// 설정이 깨져 있어도 설정 화면은 열려야 한다. 이 파일은 vim으로 직접
	// 고치는 것을 전제하므로 오타가 예상되는 실패이고, 여기서 막으면
	// 그것을 고칠 유일한 화면에 들어갈 수 없게 된다.
	c, err := config.Load(a.configPath)
	broken := err != nil
	if broken {
		a.notice("설정 파일을 읽지 못했습니다. 기본값을 보여 주지만, 값을 바꿀 때까지\n" +
			"  파일은 그대로 둡니다. 직접 고치려면 config.toml을 여세요.\n  " + err.Error())
		c = config.Default()
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

	fmt.Fprintf(a.out, "\n  %s (그대로 두려면 Enter, 취소는 :q) >> ", field.Label)
	value, _, err := ui.ReadLine(a.in)
	if err != nil {
		return err
	}
	if ui.IsCancel(value) {
		a.notice("취소했습니다. 설정은 그대로입니다.")
		return nil
	}

	updated, err := ui.ParseSetupAnswer(field.Key, value, c)
	if err != nil {
		return err
	}
	// 바뀐 것이 없으면 파일을 건드리지 않는다.
	//
	// Enter만 눌러도 저장하면, 읽지 못한 설정 파일이 기본값으로 덮어써져
	// API 키가 사라진다. 사용자는 "그대로 두려면 Enter"를 믿고 눌렀을 뿐이다.
	if updated == c {
		a.notice("바뀐 것이 없어 저장하지 않았습니다.")
		return nil
	}
	if broken {
		a.notice("읽지 못한 설정 파일을 덮어씁니다.")
	}
	if err := config.Save(a.configPath, updated); err != nil {
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

	// 각 프롬프트에서 빠져나갈 수 있어야 한다. 취소할 방법이 없으면
	// 메뉴로 돌아가려고 친 m이나 x가 레벨·주제로 들어가고, 그대로 과금되는
	// 생성이 시작된다.
	const cancelHint = " (취소는 :q)"

	dir := pack.KoToJa
	fmt.Fprint(a.out, "\n  방향 [1] 한국어→일본어  [2] 일본어→한국어"+cancelHint+" >> ")
	line, eof, err := ui.ReadLine(a.in)
	if err != nil {
		return err
	}
	if ui.IsCancel(line) || eof {
		return a.cancelFetch()
	}
	if strings.TrimSpace(line) == "2" {
		dir = pack.JaToKo
	}

	fmt.Fprintf(a.out, "  레벨 (기본 %s)%s >> ", c.Level, cancelHint)
	level, eof, err := ui.ReadLine(a.in)
	if err != nil {
		return err
	}
	if ui.IsCancel(level) || eof {
		return a.cancelFetch()
	}
	if strings.TrimSpace(level) == "" {
		level = c.Level
	}

	fmt.Fprintf(a.out, "  주제 (기본 %s)%s >> ", orNone(c.Topic), cancelHint)
	topic, eof, err := ui.ReadLine(a.in)
	if err != nil {
		return err
	}
	if ui.IsCancel(topic) || eof {
		return a.cancelFetch()
	}
	if strings.TrimSpace(topic) == "" {
		topic = c.Topic
	}

	fmt.Fprintf(a.out, "  문항 수 (기본 %d)%s >> ", c.PackSize, cancelHint)
	sizeLine, eof, err := ui.ReadLine(a.in)
	if err != nil {
		return err
	}
	if ui.IsCancel(sizeLine) || eof {
		return a.cancelFetch()
	}
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

// cancelFetch는 팩 받기를 취소한다. 과금되는 호출은 일어나지 않는다.
func (a *app) cancelFetch() error {
	a.notice("취소했습니다. 받아온 팩은 없습니다.")
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
	res, err := tsync.Run(context.Background(), client, a.dataDir, time.Now)
	if err != nil {
		return err
	}
	switch {
	case res.Processed > 0:
		a.notice(fmt.Sprintf("%d건의 첨삭을 받았습니다. 2번에서 볼 수 있습니다.", res.Processed))
	case res.Failed == 0 && res.Missing == 0:
		a.notice("처리할 항목이 없습니다.")
	}
	if res.Failed > 0 {
		msg := fmt.Sprintf("%d건은 실패해 큐에 남겼습니다. 다시 시도하면 됩니다.", res.Failed)
		if res.Aborted {
			msg += "\n  연달아 실패해서 나머지는 보내지 않았습니다. 네트워크를 확인하세요."
		}
		if res.LastError != "" {
			msg += "\n  마지막 오류: " + res.LastError
		}
		a.notice(msg)
	}
	if res.Dropped > 0 {
		msg := fmt.Sprintf("%d건은 다시 보내도 같은 이유로 실패해서 큐에서 뺐습니다. "+
			"답안은 남아 있습니다.", res.Dropped)
		if res.LastError != "" {
			msg += "\n  사유: " + res.LastError
		}
		a.notice(msg)
	}
	if res.Missing > 0 {
		a.notice(fmt.Sprintf("%d건은 문제를 찾지 못했습니다. packs/ 에서 팩이 지워졌는지 확인하세요. "+
			"답안은 큐에 그대로 남아 있습니다.", res.Missing))
	}
	return nil
}

// onlineClient는 사전 점검을 통과한 뒤에만 클라이언트를 만든다.
// 실기에서 처음 막힐 때 원인을 알려주는 것이 이 순서의 이유다.
func (a *app) onlineClient(c config.Config) (llm.Client, error) {
	if err := (llm.Preflight{}).Check(context.Background()); err != nil {
		a.online = false
		return nil, err
	}
	a.online = true
	return llm.NewAnthropic(c)
}

func (a *app) loadPackSets() ([]packSet, error) {
	var sets []packSet
	for _, d := range []pack.Direction{pack.KoToJa, pack.JaToKo} {
		ps, skipped, err := pack.LoadDir(filepath.Join(a.dataDir, "packs"), d)
		if err != nil {
			return nil, err
		}
		a.reportSkipped(skipped)
		if len(ps) == 0 {
			continue
		}
		// 키는 방향 인덱스가 아니라 세트 인덱스로 매긴다.
		// 방향 인덱스로 매기면 ko2ja 팩이 없을 때 첫 세트가 42가 되는데,
		// ParseMenuKey는 「1. 드릴 시작」을 41로 보내므로 메뉴가 죽는다.
		sets = append(sets, packSet{key: strconv.Itoa(41 + len(sets)), dir: d, problems: ps})
	}
	return sets, nil
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
	return ui.Status{Total: total, QueueLen: len(queue), Feedback: len(feedback), Online: a.online}, nil
}

// reportSkipped는 읽지 못한 팩을 알린다. 같은 파일을 두 번 알리지 않는다.
func (a *app) reportSkipped(skipped []pack.Skip) {
	for _, s := range skipped {
		if a.warned[s.Path] {
			continue
		}
		if a.warned == nil {
			a.warned = map[string]bool{}
		}
		a.warned[s.Path] = true
		a.notice(fmt.Sprintf("팩을 건너뜁니다: %s\n  %s", filepath.Base(s.Path), s.Reason))
	}
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
