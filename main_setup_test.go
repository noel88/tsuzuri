package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noel88/tsuzuri/internal/config"
	"github.com/noel88/tsuzuri/internal/pack"
)

func newAppFor(t *testing.T, dir, input string) (*app, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	return &app{
		dataDir:    dir,
		configPath: filepath.Join(dir, "config.toml"),
		termW:      92,
		in:         bufio.NewReader(strings.NewReader(input)),
		out:        &out,
		warned:     map[string]bool{},
	}, &out
}

// "그대로 두려면 Enter"를 믿고 눌렀을 뿐인데 설정이 기본값으로 덮어써져
// API 키가 사라지면 안 된다.
func TestSetupDoesNotOverwriteWhenNothingChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("api_key = \"sk-ant-real\"\nlevel = \"N2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	// 4번 항목(기본 주제)을 고르고 Enter만 누른다.
	a, _ := newAppFor(t, dir, "4\n\n")
	if err := a.setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("파일이 바뀌면 안 된다:\n전: %s\n후: %s", before, after)
	}
}

// 설정 파일이 깨져 있을 때 Enter를 눌러도 원본을 덮어쓰면 안 된다.
// 그 파일에는 아직 진짜 API 키가 들어 있을 수 있다.
func TestSetupKeepsBrokenFileUntilValueChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	broken := "api_key = \"sk-ant-real\"\nlevel = \"N3\n"
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}

	a, out := newAppFor(t, dir, "3\n\n")
	if err := a.setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	after, _ := os.ReadFile(path)
	if string(after) != broken {
		t.Errorf("깨진 파일을 덮어쓰면 안 된다:\n%s", after)
	}
	if !strings.Contains(out.String(), "파일은 그대로") {
		t.Errorf("파일을 건드리지 않았음을 알려야 한다:\n%s", out.String())
	}
}

// 메뉴로 나가려고 친 글자가 값으로 저장되면 API 키가 날아간다.
func TestSetupCancelDoesNotStoreCommandLetter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("api_key = \"sk-ant-real\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	a, _ := newAppFor(t, dir, "1\n:q\n")
	if err := a.setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-ant-real" {
		t.Errorf("API 키가 그대로여야 한다: %q", got.APIKey)
	}
}

func TestSetupSavesRealChange(t *testing.T) {
	dir := t.TempDir()
	a, _ := newAppFor(t, dir, "3\nN1\n")
	if err := a.setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Level != "N1" {
		t.Errorf("바뀐 값은 저장되어야 한다: %q", got.Level)
	}
}

// 사전 로딩은 실기에서 수십 초가 걸린다. 얼마나 걸릴지 미리 알리지 않으면
// 화면이 멈춘 것으로 오해한다.
func TestLoadDictionaryShowsEstimateAndElapsed(t *testing.T) {
	if testing.Short() {
		t.Skip("사전을 실제로 읽는다")
	}
	a, out := newAppFor(t, t.TempDir(), "")
	if _, err := a.loadDictionary(pack.KoToJa); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"사전을 읽는 중", "ko2ja", "약 17초", "완료"} {
		if !strings.Contains(s, want) {
			t.Errorf("출력에 %q가 없다:\n%s", want, s)
		}
	}
}

func TestDictLoadEstimateCoversBothDirections(t *testing.T) {
	for _, d := range []pack.Direction{pack.KoToJa, pack.JaToKo} {
		if dictLoadEstimate[d] <= 0 {
			t.Errorf("%s의 예상 시간이 없다", d)
		}
	}
}
