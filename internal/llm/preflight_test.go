package llm

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func okProbe(context.Context) error { return nil }

func sane() func() time.Time {
	return func() time.Time { return time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC) }
}

func TestCheckPassesWithSaneClockAndNetwork(t *testing.T) {
	p := Preflight{Now: sane(), Probe: okProbe}
	if err := p.Check(context.Background()); err != nil {
		t.Errorf("통과해야 한다: %v", err)
	}
}

func TestCheckRejectsAbsurdClock(t *testing.T) {
	// RTC가 밀려 1970년으로 돌아간 경우.
	p := Preflight{Now: func() time.Time { return time.Unix(0, 0) }, Probe: okProbe}
	err := p.Check(context.Background())
	if err == nil {
		t.Fatal("말이 안 되는 시각은 막아야 한다")
	}
	if !strings.Contains(err.Error(), "시각") {
		t.Errorf("시각 문제임을 알려야 한다: %v", err)
	}
}

func TestCheckDoesNotProbeWhenClockIsWrong(t *testing.T) {
	// 시각이 틀렸으면 네트워크를 건드릴 필요가 없다.
	var probed bool
	p := Preflight{
		Now:   func() time.Time { return time.Unix(0, 0) },
		Probe: func(context.Context) error { probed = true; return nil },
	}
	_ = p.Check(context.Background())
	if probed {
		t.Error("시각이 틀렸으면 네트워크를 시도하지 않아야 한다")
	}
}

func TestCheckExplainsCertificateFailureAsClockIssue(t *testing.T) {
	// 인증서 오류는 대개 시각 문제다. 그렇게 안내해야 원인을 찾는다.
	// 실제로는 url.Error로 감싸여 오므로 그 형태로 검증한다.
	wrapped := &url.Error{
		Op:  "Get",
		URL: "https://api.anthropic.com/v1/models",
		Err: x509.CertificateInvalidError{Reason: x509.Expired},
	}
	p := Preflight{Now: sane(), Probe: func(context.Context) error { return wrapped }}

	err := p.Check(context.Background())
	if err == nil {
		t.Fatal("인증서 오류를 보고해야 한다")
	}
	if !strings.Contains(err.Error(), "시각") {
		t.Errorf("시각 동기화를 안내해야 한다: %v", err)
	}
	if !strings.Contains(err.Error(), "ca-certificates") {
		t.Errorf("시각이 맞을 때의 대안도 알려야 한다: %v", err)
	}
}

func TestCheckDetectsUnknownAuthority(t *testing.T) {
	p := Preflight{Now: sane(), Probe: func(context.Context) error {
		return fmt.Errorf("연결 중: %w", x509.UnknownAuthorityError{})
	}}
	err := p.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "인증서") {
		t.Errorf("알 수 없는 인증 기관도 인증서 문제로 다뤄야 한다: %v", err)
	}
}

func TestCheckReportsPlainNetworkFailure(t *testing.T) {
	p := Preflight{Now: sane(), Probe: func(context.Context) error {
		return errors.New("no route to host")
	}}
	err := p.Check(context.Background())
	if err == nil {
		t.Fatal("네트워크 오류를 보고해야 한다")
	}
	if !strings.Contains(err.Error(), "Wi-Fi") {
		t.Errorf("네트워크 문제임을 알려야 한다: %v", err)
	}
	if strings.Contains(err.Error(), "인증서") {
		t.Errorf("인증서 문제로 오인하면 안 된다: %v", err)
	}
}
