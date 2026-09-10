# Tsuzuri (綴り)

pomera DM250 (Debian 모드)에서 오프라인으로 돌아가는 **한↔일 양방향 번역 작문 드릴**.

제시문이 한 언어로 나오면 반대 언어로 작문하고, 형태소 분석 기반 자동 분석과 LLM 첨삭으로 피드백을 받는다.

```
tsuzuri init      레벨·주제·방향 선택 → 문제 팩 생성      [온라인]
tsuzuri           출제 → 작문 → 분석                      [오프라인]
tsuzuri sync      첨삭 큐 flush + 팩 보충                 [온라인]
tsuzuri export    복습 노트를 .txt로 (순정 포메라용)      [오프라인]
```

평소에는 네트워크 없이 동작한다. 온라인이 필요한 지점은 `init`과 `sync` 둘뿐이다.

## 대상 환경

Rockchip RK3128 / 쿼드 Cortex-A7 **armhf(32비트)** / RAM 1GB / Debian 11 bullseye / fbterm 콘솔

## 빌드

맥에서 크로스컴파일한다. 포메라에는 빌드 툴체인이 필요 없다.

```bash
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o tsuzuri-armv7
```

## 설계 문서

[docs/superpowers/specs/2026-09-10-tsuzuri-design.md](docs/superpowers/specs/2026-09-10-tsuzuri-design.md)

## 브랜치 전략 (git-flow)

| 브랜치 | 역할 |
|---|---|
| `main` | 릴리스만. 태그가 붙는 곳 |
| `develop` | 기본 브랜치. 모든 기능이 여기로 병합 |
| `feature/*` | `develop`에서 분기 → `develop`으로 PR |
| `release/*` | `develop`에서 분기 → `main`과 `develop`으로 병합 |
| `hotfix/*` | `main`에서 분기 → `main`과 `develop`으로 병합 |

`main`으로의 직접 푸시는 하지 않는다.
