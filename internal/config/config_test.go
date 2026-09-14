package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "없음.toml"))
	if err != nil {
		t.Fatalf("없는 파일은 오류가 아니어야 한다: %v", err)
	}
	if got.Model != "claude-opus-5" {
		t.Errorf("기본 모델 = %q", got.Model)
	}
	if got.PackSize <= 0 {
		t.Errorf("기본 팩 크기가 있어야 한다: %d", got.PackSize)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	want := Config{APIKey: "sk-ant-test", Model: "claude-sonnet-5", Level: "N3", Topic: "일상", PackSize: 50}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Errorf("왕복 실패:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestSaveIsHandEditable(t *testing.T) {
	// 포메라에서 vim으로 고칠 파일이다. 사람이 읽을 수 있어야 한다.
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Config{Model: "claude-opus-5", Level: "N3"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	if !strings.Contains(s, "model") || !strings.Contains(s, "claude-opus-5") {
		t.Errorf("키와 값이 평문으로 보여야 한다:\n%s", s)
	}
}

func TestSaveRestrictsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Config{APIKey: "sk-ant-test"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("권한 = %v, 기대 0600", fi.Mode().Perm())
	}
}

func TestLoadBrokenFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("이건 = TOML이 아니다 ==="), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("깨진 설정은 조용히 넘어가면 안 된다")
	}
}

func TestResolvedKeyPrefersEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-from-env")
	c := Config{APIKey: "sk-ant-from-file"}
	if got := c.ResolvedKey(); got != "sk-ant-from-env" {
		t.Errorf("환경변수가 우선이어야 한다: %q", got)
	}
}

func TestResolvedKeyFallsBackToFile(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	c := Config{APIKey: "sk-ant-from-file"}
	if got := c.ResolvedKey(); got != "sk-ant-from-file" {
		t.Errorf("파일 값을 써야 한다: %q", got)
	}
}

func TestValidateRejectsMissingKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if err := (Config{Model: "claude-opus-5"}).Validate(); err == nil {
		t.Error("키가 없으면 오류여야 한다")
	}
}

func TestValidatePassesWithKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if err := (Config{APIKey: "sk-ant-test", Model: "claude-opus-5"}).Validate(); err != nil {
		t.Errorf("통과해야 한다: %v", err)
	}
}
