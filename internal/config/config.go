package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

// Config는 config.toml의 내용이다.
//
// 이 파일은 SD카드의 vfat 영역에 놓이므로 API 키가 평문으로 남는다.
// 기기나 SD를 잃으면 노출된다 — 전용 키를 쓰고 사용량 제한을 걸어야 한다.
// 설정 화면에서 사용자에게 이 사실을 알린다 (스펙 §7.5).
type Config struct {
	APIKey   string `toml:"api_key"`
	Model    string `toml:"model"`
	Level    string `toml:"level"`
	Topic    string `toml:"topic"`
	PackSize int    `toml:"pack_size"`
}

// Default는 설정 파일이 없을 때의 값이다.
func Default() Config {
	return Config{
		Model:    "claude-opus-5",
		Level:    "N3",
		PackSize: 50,
	}
}

// Load는 설정을 읽는다. 파일이 없으면 기본값을 돌려준다 — 오류가 아니다.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return c, err
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		// 날 파싱 오류만 보여 주면 나갈 길이 안 보인다. 이 파일이 깨지면
		// 설정을 읽는 기능이 전부 막히므로, 고치는 방법을 함께 말한다.
		return Default(), fmt.Errorf("설정 파일이 깨졌습니다: %s\n"+
			"  설정 화면에서 값을 다시 넣거나, 이 파일을 지우면 기본값으로 시작합니다: %s",
			err, path)
	}
	if c.Model == "" {
		c.Model = Default().Model
	}
	if c.PackSize <= 0 {
		c.PackSize = Default().PackSize
	}
	return c, nil
}

// Save는 설정을 쓴다.
//
// 임시 파일에 쓰고 rename으로 바꾼다. 제자리에서 truncate하면 그 사이에
// 전원이 끊길 때 API 키를 잃는데, 포메라는 예고 없이 꺼지는 기계다.
//
// 권한은 0600으로 맞춰 본다. OpenFile의 mode는 파일을 **만들 때만** 적용되어
// vim으로 먼저 만들어 둔 0644 파일은 그냥 두면 세계 읽기 가능인 채로 API 키를
// 담게 되기 때문이다.
//
// 다만 권한 설정 실패를 치명적으로 다루지 않는다. 포메라의 데이터는 vfat
// SD카드에 놓이는데, vfat은 유닉스 권한 자체가 없어 chmod가 거부될 수 있다.
// 그것을 오류로 처리하면 그 기기에서는 설정을 아예 저장하지 못한다.
// vfat에서는 어차피 권한을 좁힐 수 없으므로, 키를 숨기려면 파일 대신
// ANTHROPIC_API_KEY 환경변수를 쓰는 것이 올바른 방법이다.
func Save(path string, c Config) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// 실패해도 계속 진행한다 (vfat 등 권한을 지원하지 않는 파일 시스템).
	_ = os.Chmod(tmp, 0o600)

	return os.Rename(tmp, path)
}

// ResolvedKeyOf는 실제로 쓸 API 키를 고른다.
// 환경변수가 파일보다 우선이다 — SD카드에 키를 두고 싶지 않으면
// 환경변수만 쓰면 된다.
func ResolvedKeyOf(fileKey string) string {
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		return v
	}
	return fileKey
}

func (c Config) ResolvedKey() string { return ResolvedKeyOf(c.APIKey) }

// Validate는 온라인 작업을 시작할 수 있는 상태인지 본다.
func (c Config) Validate() error {
	if c.ResolvedKey() == "" {
		return errors.New("API 키가 없습니다. config.toml의 api_key 또는 ANTHROPIC_API_KEY를 설정하세요")
	}
	if c.Model == "" {
		return errors.New("model이 비어 있습니다")
	}
	return nil
}
