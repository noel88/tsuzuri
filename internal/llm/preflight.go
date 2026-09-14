package llm

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// clockFloor는 이보다 이전 시각이면 RTC가 밀린 것으로 본다.
// 이 프로젝트가 시작된 시점보다 앞설 수 없다.
var clockFloor = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Preflight는 온라인 작업 전에 흔한 실패 원인을 먼저 걸러낸다.
//
// 포메라는 Debian을 가끔만 띄우므로 시각이 밀려 있을 수 있다.
// 시각이 어긋나면 TLS 인증서 검증이 실패하는데, 그때 나오는 에러만으로는
// 원인을 짐작하기 어렵다. 먼저 보고 먼저 알려준다 (스펙 §7.6).
type Preflight struct {
	Now   func() time.Time
	Probe func(ctx context.Context) error
}

// Check는 시각과 네트워크를 확인한다.
func (p Preflight) Check(ctx context.Context) error {
	now := p.Now
	if now == nil {
		now = time.Now
	}
	if t := now(); t.Before(clockFloor) {
		return fmt.Errorf("시스템 시각이 %s로 되어 있습니다. "+
			"시각이 틀리면 TLS 인증서 검증이 실패합니다. "+
			"`sudo date -s '<현재 시각>'` 또는 NTP로 맞춰 주세요",
			t.Format("2006-01-02 15:04"))
	}

	probe := p.Probe
	if probe == nil {
		probe = DefaultProbe
	}
	if err := probe(ctx); err != nil {
		if isCertError(err) {
			return fmt.Errorf("TLS 인증서 검증에 실패했습니다. "+
				"시스템 시각(%s)이 맞는지 먼저 확인하세요. "+
				"시각이 맞다면 `sudo apt install --reinstall ca-certificates`를 시도하세요: %w",
				now().Format("2006-01-02 15:04"), err)
		}
		return fmt.Errorf("네트워크에 닿지 않습니다. Wi-Fi 연결을 확인하세요: %w", err)
	}
	return nil
}

func isCertError(err error) bool {
	var invalid x509.CertificateInvalidError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	return errors.As(err, &invalid) || errors.As(err, &unknown) || errors.As(err, &hostname)
}

// DefaultProbe는 인증 없이 API 엔드포인트에 닿아 TLS만 확인한다.
// 401이 돌아와도 성공이다 — 인증만 없을 뿐 연결과 TLS는 된 것이다.
func DefaultProbe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
