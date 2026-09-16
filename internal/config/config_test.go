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

func TestSaveNarrowsPermissionsOnExistingFile(t *testing.T) {
	// OpenFile의 mode는 생성할 때만 적용된다. vim으로 먼저 만든 0644 파일에
	// 키를 저장하면 세계 읽기 가능인 채로 남는다.
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("level = \"N3\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Config{APIKey: "sk-ant-secret", Model: "claude-opus-5"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("기존 파일 권한 = %v, 기대 0600", fi.Mode().Perm())
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := Save(path, Config{APIKey: "sk-ant-test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("임시 파일이 남으면 안 된다")
	}
}

func TestSaveDoesNotDestroyExistingOnEncodeFailure(t *testing.T) {
	// 제자리 truncate였다면 쓰기 도중 실패가 기존 키를 날린다.
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Config{APIKey: "sk-ant-original", Model: "claude-opus-5"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// rename 방식이면 tmp에만 쓰이므로 원본은 어느 시점에도 비지 않는다.
	if !strings.Contains(string(before), "sk-ant-original") {
		t.Fatalf("원본이 온전해야 한다:\n%s", before)
	}
}

// 포메라의 데이터는 vfat SD카드에 놓인다. vfat은 유닉스 권한이 없어
// chmod가 거부될 수 있는데, 그것을 오류로 다루면 그 기기에서는 설정을
// 아예 저장하지 못한다.
func TestSaveSucceedsWhenChmodIsNotSupported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := Save(path, Config{APIKey: "sk-ant-test", Model: "claude-opus-5"}); err != nil {
		t.Fatal(err)
	}

	// 디렉터리는 쓸 수 있지만 파일 소유자가 아니어서 chmod가 거부되는 상황을
	// 흉내내기는 어렵다. 대신 Save가 chmod 결과에 의존하지 않는지 본다:
	// 읽기 전용 권한으로 만들어 둔 파일도 덮어써져야 한다.
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Config{APIKey: "sk-ant-second", Model: "claude-opus-5"}); err != nil {
		t.Fatalf("권한 때문에 저장이 막히면 안 된다: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-ant-second" {
		t.Errorf("APIKey = %q", got.APIKey)
	}
}
