package ui

import (
	"fmt"
	"testing"

	"github.com/noel88/tsuzuri/internal/analyze"
	"github.com/noel88/tsuzuri/internal/pack"
)

// TestLiveScreen은 목업이 아니라 실제 분석기 결과로 화면을 그린다.
//
//	go test ./internal/ui/ -run TestLiveScreen -v
func TestLiveScreen(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("출력 확인용이다. -v 로 실행한다")
	}
	az, err := analyze.New(pack.KoToJa)
	if err != nil {
		t.Fatal(err)
	}
	p := sampleProblem()
	st := Status{Index: 12, Total: 47, QueueLen: 12, Online: false}

	fmt.Print("\n### 출제 화면\n")
	fmt.Print(RenderProblem(p, st, 92))

	for _, ans := range []string{
		"昨日初めて行ったカフェは静かくて、ずっと座ってました。",
		"昨日初めて行ったカフェが思ったより静かで、長く座っていた。",
	} {
		fmt.Printf("\n### 결과 화면 — 답안: %s\n", ans)
		fmt.Print(RenderResult(p, ans, az.Analyze(p, ans), st, 92, 0))
	}
}
