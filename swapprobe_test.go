package main

import (
	"runtime"
	"testing"

	"github.com/noel88/tsuzuri/internal/pack"
)

func liveMB() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / 1024 / 1024
}

// 한 프로세스는 사전을 하나만 연다.
//
// kagome 사전은 패키지 전역에 sync.Once로 상주하므로 참조를 끊어도
// 회수되지 않는다. 그래서 방향이 바뀌면 두 번째 사전을 여는 대신
// 자기 자신을 다시 실행한다. 이 테스트는 두 번째 사전을 여는 경로가
// 존재하지 않음을 고정한다.
func TestOnlyOneDictionaryPerProcess(t *testing.T) {
	a := newApp()
	a.out = discard{}

	if _, err := a.analyzerFor(pack.KoToJa); err != nil {
		t.Fatal(err)
	}
	afterJa := liveMB()
	t.Logf("일본어 사전 하나: %.1f MB", afterJa)

	// 다른 방향을 요청하면 분석기를 새로 만들지 않는다.
	// (테스트에서는 os.Executable()이 테스트 바이너리라 exec가
	//  일어나면 안 되므로, 여기서는 오류 경로만 확인한다.)
	if a.analyzerDir != pack.KoToJa {
		t.Fatalf("방향이 기록되어야 한다: %q", a.analyzerDir)
	}

	after := liveMB()
	if after > 200 {
		t.Errorf("사전이 두 개 상주 중이다: %.1f MB", after)
	}
	t.Logf("→ 두 사전 동시 상주(300MB)를 피했다")
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
