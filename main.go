// Command tsuzuri는 포메라 DM250에서 오프라인으로 도는
// 한↔일 양방향 번역 작문 드릴이다.
//
// 네트워크는 팩을 받을 때(8)와 첨삭을 받을 때(9)만 쓴다.
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
	"github.com/noel88/tsuzuri/internal/progress"
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

	// 아직 사용자가 읽지 않은 메시지가 화면에 있는지.
	noticed bool

	// 초기화면에서 화살표가 가리키는 자리. 화면을 다녀와도 그 자리가
	// 남아 있어야 한다 — 매번 맨 위로 돌아가면 같은 곳을 반복해서 못 간다.
	cursor ui.Cursor

	// 키를 하나씩 읽는 읽개. 화살표를 쓸 수 없는 입력이면 비어 있다.
	keys *ui.KeyReader

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
	a := &app{
		dataDir:    dataDir,
		configPath: filepath.Join(dataDir, "config.toml"),
		termW:      ui.TermWidth(),
		in:         bufio.NewReader(os.Stdin),
		out:        os.Stdout,
		warned:     map[string]bool{},
	}
	if ui.Interactive() {
		a.keys = ui.NewKeyReader(os.Stdin)
	}
	return a
}

// packSet은 팩 파일 하나다. 초기화면의 자료실 항목 하나에 대응한다.
//
// 방향별로 묶지 않고 파일별로 둔다. 방향으로 묶으면 자료실 한 줄이 그
// 방향의 모든 팩을 뭉뚱그리게 되고, 레벨과 주제가 섞여 나온다 — 실기에서
// N2로 받은 팩이 시작 팩과 합쳐져 「2·N3」으로 보였다. 사용자는 그 줄을
// 「방금 받은 팩」으로 읽는데 실제로는 「이 방향의 전부」였다.
type packSet struct {
	key      string
	dir      pack.Direction
	source   string
	problems []pack.Problem
}

func (p packSet) label() string {
	return fmt.Sprintf("%s  %s  %s", p.dir, one(p.levels(), "여러 레벨"), one(p.topics(), "여러 주제"))
}

func (p packSet) levels() []string {
	return uniq(p.problems, func(pr pack.Problem) string { return pr.Level })
}
func (p packSet) topics() []string {
	return uniq(p.problems, func(pr pack.Problem) string { return pr.Topic })
}

// one은 값이 하나뿐이면 그것을, 여러 개면 대신할 말을 돌려준다.
//
// 값을 두어 개 이어 붙여 보여주면 그 줄이 팩 전체를 말하는 것처럼 읽힌다.
// 25문항이 스물두 가지 주제를 담고 있으면 앞의 둘은 대표가 아니다.
func one(vs []string, many string) string {
	switch len(vs) {
	case 0:
		return "-"
	case 1:
		return vs[0]
	default:
		return many
	}
}

func uniq(ps []pack.Problem, key func(pack.Problem) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range ps {
		v := key(p)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
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
			var ok bool
			key, eof, ok = a.chooseMenu(choices, st)
			if !ok {
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
		if a.pause() {
			return nil
		}
	}
}

// chooseMenu는 초기화면을 그리고 어디로 갈지 고르게 한다.
//
// 화살표로 옮기고 Enter로 고르거나, 번호를 쳐서 바로 간다. 둘 다 남겨 둔
// 것은 자료실 번호가 두 자리여서 키 하나로는 고를 수 없기 때문이고,
// 기기에서 화살표가 안 먹히더라도 앱을 쓸 수 있어야 하기 때문이다.
func (a *app) chooseMenu(choices []ui.Choice, st ui.Status) (key string, eof, ok bool) {
	if a.keys == nil {
		ui.Clear(a.out)
		fmt.Fprint(a.out, ui.RenderMenu(choices, st, a.termW, a.cursor))
		line, isEOF, err := ui.ReadLine(a.in)
		if err != nil {
			return "", true, false
		}
		k, cont := ui.ParseMenuKey(line)
		return k, isEOF, cont
	}

	// typed는 지금까지 친 번호다. 치는 동안 프롬프트 뒤에 보여 준다.
	typed := ""
	for {
		a.cursor = ui.ClampCursor(a.cursor, choices, st)
		ui.Clear(a.out)
		fmt.Fprint(a.out, ui.RenderMenu(choices, st, a.termW, a.cursor), typed)

		k, err := a.keys.Read()
		if err != nil {
			// raw mode가 안 되는 터미널이다. 한 줄 읽기로 물러난다.
			line, isEOF, err := ui.ReadLine(a.in)
			if err != nil {
				return "", true, false
			}
			kk, cont := ui.ParseMenuKey(line)
			return kk, isEOF, cont
		}

		switch k.Key {
		case ui.KeyQuit:
			return "", true, false
		case ui.KeyUp, ui.KeyDown, ui.KeyLeft, ui.KeyRight:
			// 번호를 치던 중에 화살표를 누르면 치던 것을 버린다.
			// 마음을 바꾼 것이지 두 가지를 섞으려는 것이 아니다.
			typed = ""
			a.cursor = ui.MoveCursor(a.cursor, k.Key, choices, st)
		case ui.KeyEnter:
			if typed != "" {
				kk, cont := ui.ParseMenuKey(typed)
				return kk, false, cont
			}
			if kk, found := ui.MenuKeyAt(choices, st, a.cursor); found {
				return kk, false, true
			}
		case ui.KeyEscape:
			typed = ""
		case ui.KeyRune:
			switch {
			case k.Rune == 0x7f || k.Rune == 0x08: // 지우기
				if typed != "" {
					typed = typed[:len(typed)-1]
				}
			case k.Rune >= '0' && k.Rune <= '9':
				typed += string(k.Rune)
			default:
				if kk, cont := ui.ParseMenuKey(string(k.Rune)); !cont {
					return kk, false, false
				}
			}
		}
	}
}

// dispatch는 초기화면의 선택을 처리한다. 종료해야 하면 true를 돌려준다.
func (a *app) dispatch(key string, sets []packSet) (bool, error) {
	switch key {
	case "2":
		return a.resume(sets)
	case "3":
		return a.reviewDrill(sets)
	case "4":
		return a.review(sets)
	case "5":
		return a.stats(sets)
	case "6":
		return false, a.export()
	case "7":
		return false, a.setup()
	case "8":
		return false, a.fetchPack()
	case "9":
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
	return a.drillFrom(set.problems, set.dir, 0)
}

// drillFrom은 주어진 문제들을 start번째부터 낸다.
func (a *app) drillFrom(problems []pack.Problem, dir pack.Direction, start int) (bool, error) {
	az, err := a.analyzerFor(dir)
	if err != nil {
		return false, err
	}

	s := &drill.Session{
		Problems: problems,
		Start:    start,
		Analyzer: az,
		DataDir:  a.dataDir,
		In:       a.in,
		Out:      a.out,
		TermW:    a.termW,
		Online:   a.online,
		Keys:     os.Stdin,
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
	key, _ := a.keyForDir(d)
	return a.restartTo(d, key)
}

// restartTo는 다시 실행한 뒤 gotoKey 화면을 연다.
func (a *app) restartTo(d pack.Direction, gotoKey string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("방향을 바꾸려면 앱을 다시 실행해야 합니다: %w", err)
	}
	a.notice(fmt.Sprintf("방향이 바뀌어 다시 시작합니다 (%s → %s)...", a.analyzerDir, d))

	env := os.Environ()
	if gotoKey != "" {
		env = append(env, gotoEnv+"="+gotoKey)
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

// known은 지금 자료실에 있는 문제를 ID로 찾을 수 있게 모은다.
//
// 팩 파일은 사용자가 지울 수 있다. 지운 팩의 문제는 답안이 남아 있어도
// 다시 낼 수 없고 첨삭을 보여줄 수도 없다 — 제시문이 그 파일에 있었다.
func known(sets []packSet) map[string]pack.Problem {
	out := map[string]pack.Problem{}
	for _, s := range sets {
		for _, p := range s.problems {
			out[p.ID] = p
		}
	}
	return out
}

// resume은 마지막으로 푼 문제의 다음부터 이어 낸다.
//
// 어디까지 했는지를 따로 저장하지 않는다. 마지막 답안이 곧 그 자리다 —
// 따로 저장하면 전원이 끊긴 자리에서 둘이 어긋난다.
func (a *app) resume(sets []packSet) (bool, error) {
	attempts, err := store.ReadAll[store.Attempt](filepath.Join(a.dataDir, "attempts.jsonl"))
	if err != nil {
		return false, err
	}
	last, ok := progress.LastPack(attempts)
	if !ok {
		a.notice("아직 푼 문제가 없습니다. 1번으로 시작하세요.")
		return false, nil
	}

	for _, set := range sets {
		for i, p := range set.problems {
			if p.ID != last {
				continue
			}
			if i+1 >= len(set.problems) {
				a.notice(fmt.Sprintf("%s 팩은 끝까지 풀었습니다. 처음부터 다시 냅니다.", set.label()))
				return a.drillFrom(set.problems, set.dir, 0)
			}
			return a.drillFrom(set.problems, set.dir, i+1)
		}
	}
	a.notice("마지막으로 푼 문제가 있던 팩이 보이지 않습니다. 팩을 지웠다면 1번으로 새로 시작하세요.")
	return false, nil
}

// reviewDrill은 복습할 문제만 모아서 낸다.
//
// 목록은 첨삭에서 오류가 나온 문제와 사용자가 B로 표시한 문제다.
// 맞혔는지는 다음 첨삭이 판정한다 — 스스로 채점하게 하지 않는다.
func (a *app) reviewDrill(sets []packSet) (bool, error) {
	cards, _, _, err := a.progressData(sets)
	if err != nil {
		return false, err
	}
	due := progress.Due(cards)
	if len(due) == 0 {
		a.notice("복습할 문제가 없습니다.\n" +
			"  첨삭에서 오류가 나온 문제와, 결과 화면에서 B로 표시한 문제가 여기 모입니다.")
		return false, nil
	}

	// 방향별로 나눈다. 한 회차에 두 방향을 섞을 수 없다 — 사전이 하나뿐이다.
	byID := known(sets)
	groups := map[pack.Direction][]pack.Problem{}
	for _, id := range due {
		if p, ok := byID[id]; ok {
			groups[p.Dir] = append(groups[p.Dir], p)
		}
	}

	dir := a.reviewDir(groups)
	// 이미 열어 둔 사전과 방향이 다르면 다시 실행한다. 돌아온 뒤 곧바로
	// 이 화면을 다시 연다 — 사용자가 메뉴에서 3을 또 누르게 하지 않는다.
	if a.analyzer != nil && a.analyzerDir != dir {
		return false, a.restartTo(dir, "3")
	}

	quit, err := a.drillFrom(groups[dir], dir, 0)
	if err != nil || quit {
		return quit, err
	}
	// 남은 방향은 회차가 끝난 뒤에 알린다. 드릴이 화면을 지우므로 먼저
	// 띄우면 읽을 새도 없이 덮인다.
	if other := len(due) - len(groups[dir]); other > 0 {
		a.notice(fmt.Sprintf("다른 방향에 복습할 문제가 %d개 남았습니다. 3번을 다시 고르면 그쪽을 냅니다.", other))
	}
	return false, nil
}

// reviewDir은 이번 복습 회차의 방향을 고른다.
//
// 이미 읽어 둔 사전이 있으면 그쪽을 먼저 쓴다. 방향을 바꾸면 사전을 다시
// 읽어야 하고 그것이 실기에서 16.5초와 39초다 — 복습 두 개를 풀자고
// 치를 값이 아니다. 열어 둔 사전에 낼 것이 없을 때만 바꾼다.
func (a *app) reviewDir(groups map[pack.Direction][]pack.Problem) pack.Direction {
	if a.analyzer != nil && len(groups[a.analyzerDir]) > 0 {
		return a.analyzerDir
	}
	best, n := pack.KoToJa, -1
	for _, d := range []pack.Direction{pack.KoToJa, pack.JaToKo} {
		if len(groups[d]) > n {
			best, n = d, len(groups[d])
		}
	}
	return best
}

// stats는 진도 화면이다.
func (a *app) stats(sets []packSet) (bool, error) {
	cards, attempts, feedback, err := a.progressData(sets)
	if err != nil {
		return false, err
	}
	now := time.Now()
	s := progress.Summarize(attempts, feedback, cards, now)

	labels := map[string]string{}
	for id, p := range known(sets) {
		labels[id] = ui.TruncateMark(p.Prompt, 40)
	}

	ui.Clear(a.out)
	fmt.Fprint(a.out, ui.RenderStats(s, cards, labels, ui.Status{Online: a.online, Now: now}, a.termW))

	line, eof, err := ui.ReadLine(a.in)
	if err != nil {
		return false, err
	}
	if eof || ui.ParseCommand(line) == ui.CmdQuit {
		return true, nil
	}
	return false, nil
}

// progressData는 진도 계산에 필요한 것을 한 번에 읽는다.
//
// 자료실에 없는 문제의 카드는 버린다. 팩을 지우면 그 문제는 낼 수 없는데,
// 세기만 하면 초기화면이 「복습 드릴 (1)」이라고 해 놓고 들어가면 낼 것이
// 없다고 답하게 된다.
func (a *app) progressData(sets []packSet) ([]progress.Card, []store.Attempt, []tsync.Feedback, error) {
	attempts, err := store.ReadAll[store.Attempt](filepath.Join(a.dataDir, "attempts.jsonl"))
	if err != nil {
		return nil, nil, nil, err
	}
	feedback, err := store.ReadAll[tsync.Feedback](filepath.Join(a.dataDir, "feedback.jsonl"))
	if err != nil {
		return nil, nil, nil, err
	}
	marks, err := store.ReadAll[progress.Mark](filepath.Join(a.dataDir, "marks.jsonl"))
	if err != nil {
		return nil, nil, nil, err
	}
	byID := known(sets)
	var cards []progress.Card
	for _, c := range progress.Cards(attempts, feedback, marks, time.Now()) {
		if _, ok := byID[c.ProblemID]; ok {
			cards = append(cards, c)
		}
	}
	return cards, attempts, feedback, nil
}

// review는 받은 첨삭을 순서대로 보여준다.
func (a *app) review(sets []packSet) (bool, error) {
	feedback, err := store.ReadAll[tsync.Feedback](filepath.Join(a.dataDir, "feedback.jsonl"))
	if err != nil {
		return false, err
	}
	if len(feedback) == 0 {
		a.notice("받은 첨삭이 없습니다. 9번으로 첨삭을 받아 오세요.")
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
	problems := known(sets)

	// 보여줄 수 있는 것만 먼저 고른다.
	//
	// 걸러 놓고 세지 않으면 「3건 중 2번째」처럼 있지도 않은 번호가 뜬다.
	// 하나도 못 보여줄 때 조용히 메뉴로 돌아가는 것도 여기서 막는다 —
	// 「받은 첨삭 1건」이라고 써 놓고 눌러도 아무 일이 없으면 고장으로 읽힌다.
	type shown struct {
		f  tsync.Feedback
		at store.Attempt
		p  pack.Problem
	}
	var list []shown
	dropped := 0
	for _, f := range feedback {
		at, ok := byID[f.AttemptID]
		if !ok {
			dropped++
			continue
		}
		p, ok := problems[at.PackID]
		if !ok {
			// 팩이 사라졌다. 제시문 없이 첨삭만 보여주면 맥락이 없다.
			dropped++
			continue
		}
		list = append(list, shown{f, at, p})
	}
	if dropped > 0 {
		a.notice(fmt.Sprintf("%d건은 문제를 찾지 못해 건너뜁니다. 팩을 지우면 그 첨삭은 볼 수 없습니다.", dropped))
	}
	if len(list) == 0 {
		return false, nil
	}

	for i, it := range list {
		f, at, p := it.f, it.at, it.p
		st := ui.Status{Index: i + 1, Total: len(list), Online: a.online}
		ui.Clear(a.out)
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
	ui.Clear(a.out)
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

	dirs := []pack.Direction{pack.KoToJa}
	fmt.Fprint(a.out, "\n  방향 [1] 한국어→일본어  [2] 일본어→한국어  [3] 둘 다"+cancelHint+" >> ")
	line, eof, err := ui.ReadLine(a.in)
	if err != nil {
		return err
	}
	if ui.IsCancel(line) || eof {
		return a.cancelFetch()
	}
	switch strings.TrimSpace(line) {
	case "2":
		dirs = []pack.Direction{pack.JaToKo}
	case "3":
		dirs = []pack.Direction{pack.KoToJa, pack.JaToKo}
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
	// 설정 화면과 같은 규칙으로 고친다. 여기서 안 고치면 "2" 가 그대로
	// 생성 프롬프트에 들어가고 팩에도 그렇게 박힌다.
	level = pack.NormalizeLevel(strings.TrimSpace(level))

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

	sizeLabel := fmt.Sprintf("기본 %d", c.PackSize)
	if len(dirs) > 1 {
		sizeLabel += ", 두 방향이 나눠 가집니다"
	}
	fmt.Fprintf(a.out, "  문항 수 (%s)%s >> ", sizeLabel, cancelHint)
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

	// 방향을 둘 다 고르면 팩도 둘로 만든다.
	//
	// 한 팩에 두 방향을 섞을 수는 없다. 방향마다 다른 사전이 필요한데 한
	// 프로세스는 사전을 하나만 열 수 있어서(analyzerFor 참고), 문제마다
	// 방향이 바뀌면 그때마다 사전을 다시 읽어야 한다 — 실기에서 16.5초와
	// 39초다. 받는 일만 한 번에 끝내고, 푸는 것은 방향별로 한다.
	for i, dir := range dirs {
		spec := gen.Spec{
			Dir:   dir,
			Level: level,
			Topic: strings.TrimSpace(topic),
			Count: share(count, len(dirs), i),
		}
		a.notice(fmt.Sprintf("%s %d문항을 만드는 중입니다. 몇 분 걸릴 수 있습니다...",
			dirLabel(dir), spec.Count))

		ps, err := gen.Generate(context.Background(), client, spec)
		if err != nil {
			// 앞의 방향이 이미 저장됐으면 그것은 남는다. 두 번째가 실패했다고
			// 첫 번째까지 버리면 그 호출은 이미 과금된 뒤다.
			return err
		}
		path, err := gen.WritePack(filepath.Join(a.dataDir, "packs"), spec, ps, time.Now())
		if err != nil {
			return err
		}
		a.notice(fmt.Sprintf("%d문항을 받았습니다: %s", len(ps), filepath.Base(path)))
	}
	return nil
}

// share는 총 문항 수를 방향 수만큼 나눈다. 나머지는 앞쪽이 가진다.
// 방향마다 최소 한 문항은 준다 — 0문항짜리 호출은 돈만 쓰고 빈 팩을 만든다.
func share(total, parts, i int) int {
	n := total / parts
	if i < total%parts {
		n++
	}
	if n < 1 {
		n = 1
	}
	return n
}

func dirLabel(d pack.Direction) string {
	if d == pack.JaToKo {
		return "일→한"
	}
	return "한→일"
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
		a.notice(fmt.Sprintf("%d건의 첨삭을 받았습니다. 4번에서 볼 수 있습니다.", res.Processed))
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

		// 같은 파일에서 온 것끼리 묶는다. LoadDir이 파일 순서대로 주므로
		// 자리는 매번 같다 — 어제 41이던 팩이 오늘 43이 되면 안 된다.
		for i := 0; i < len(ps); {
			j := i
			for j < len(ps) && ps[j].Source == ps[i].Source {
				j++
			}
			// 키는 방향 인덱스가 아니라 세트 인덱스로 매긴다.
			// 방향 인덱스로 매기면 ko2ja 팩이 없을 때 첫 세트가 42가 되는데,
			// ParseMenuKey는 「1. 드릴 시작」을 41로 보내므로 메뉴가 죽는다.
			sets = append(sets, packSet{
				key:      strconv.Itoa(41 + len(sets)),
				dir:      d,
				source:   ps[i].Source,
				problems: ps[i:j],
			})
			i = j
		}
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

	// 복습할 것이 몇 개인지는 초기화면에서 보여야 한다. 들어가 봐야 알면
	// 「오늘 할 게 있나」를 확인하려고 매번 그 화면을 열게 된다.
	cards, _, _, err := a.progressData(sets)
	if err != nil {
		return ui.Status{}, err
	}

	return ui.Status{
		Total:    total,
		QueueLen: len(queue),
		Feedback: len(feedback),
		Due:      len(progress.Due(cards)),
		Online:   a.online,
	}, nil
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
	fmt.Fprintf(a.out, "\n  %s\n", msg)
	a.noticed = true
}

// pause는 화면을 지우기 전에 읽을 틈을 준다.
//
// 화면 지우기를 넣은 뒤로, 메시지를 띄우고 곧바로 다음 화면을 그리면
// 그 메시지가 덮여 사라진다. 「저장했습니다」나 오류 문구를 아무도 못 읽는다.
// 화면을 지우지 않는 설정이면 메시지가 그대로 남으므로 멈추지 않는다.
//
// 입력이 끝났으면(EOF) true를 돌려준다 — 그러지 않으면 메뉴가 빈 입력을
// 계속 받아 되돌아오며 무한히 돈다.
func (a *app) pause() bool {
	if !a.noticed {
		return false
	}
	a.noticed = false
	if !ui.ClearEnabled() {
		return false
	}
	fmt.Fprint(a.out, "\n  계속하려면 Enter >> ")
	_, eof, err := ui.ReadLine(a.in)
	return eof || err != nil
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
