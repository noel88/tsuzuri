# 실기(포메라 DM250)로 옮기기

## 옮길 파일

**바이너리 하나면 동작한다.** 형태소 사전이 바이너리에 임베드돼 있어
별도 데이터 파일이 필요 없다.

| 파일 | 필요성 | 설명 |
|---|---|---|
| `tsuzuri` | **필수** | armv7 정적 바이너리 (46MB) |
| `packs/*.jsonl` | 선택 | 문제 팩. 없으면 앱 안에서 `5. 팩 받기`로 받는다 |
| `config.toml` | 선택 | 없으면 기본값. API 키는 앱 안 `4. 설정`에서도 넣을 수 있다 |
| `m0-check.sh` | 검증용 | 실기 확인 스크립트 |

나머지(`attempts.jsonl`, `queue.jsonl`, `feedback.jsonl`, `review/`)는
앱이 실행 중에 만든다.

## 바이너리 만들기

```bash
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags="-s -w" -o deploy/tsuzuri .
```

`-ldflags="-s -w"`는 디버그 심볼을 빼 51MB를 46MB로 줄인다. 크래시를
추적해야 하면 이 플래그를 빼고 빌드한다.

## SD카드 배치

```
<SD 마운트 지점>/tsuzuri/
  tsuzuri              ← 바이너리 (chmod +x)
  m0-check.sh
  config.toml          ← config.toml.example을 복사해 수정
  packs/
    sample-ko2ja.jsonl ← testdata/packs/ 에서 복사 (맛보기용)
    sample-ja2ko.jsonl
```

마운트 지점은 설치마다 다르다. 기기에서 `df -h`로 확인한다.

## 옮기는 방법

**SD카드를 직접 꽂아서** — 가장 확실하다. 맥에서 SD에 쓰고 포메라에 꽂는다.

**Tailscale로** — 포메라가 tailnet에 있으면:

```bash
scp deploy/tsuzuri deploy/m0-check.sh <포메라>:~/tsuzuri/
scp testdata/packs/*.jsonl <포메라>:~/tsuzuri/packs/
```

## 실행

```bash
cd <tsuzuri 디렉터리>
chmod +x tsuzuri m0-check.sh
./tsuzuri
```

터미널 폭은 자동으로 감지한다(TIOCGWINSZ). 감지가 안 되는 환경이라면
`COLUMNS`를 export해서 넘길 수 있다:

```bash
export COLUMNS=92 && ./tsuzuri
```

데이터 위치를 따로 두려면:

```bash
TSUZURI_DATA=/mnt/sd/tsuzuri ./tsuzuri
```

## 처음 할 일: M0 검증

```bash
sh m0-check.sh
```

스펙 §9의 R1~R6을 한 번에 확인한다. 결과를 기록해 두면 남은 설계 결정
(방향별 바이너리 분리 여부, ASCII 폴백 필요 여부)을 내릴 수 있다.

특히 **R2(한글 입력)** 는 스크립트가 대신 못 하므로 손으로 확인해야 한다.
`ja2ko` 방향(한국어로 작문)이 여기에 달려 있다.

## 알아둘 것

- **방향을 바꾸면 앱이 자동으로 재시작한다.** kagome 사전이 프로세스가
  끝날 때까지 메모리에 상주하기 때문이다(일본어 88MB, 한국어 211MB).
  둘을 동시에 들면 RAM 1GB 기기에서 OOM이므로, 자기 자신을 다시 실행해
  OS가 회수하게 한다. 재시작 후 고르려던 팩이 바로 열린다.
- **API 키는 SD카드에 평문으로 남는다.** 전용 키를 발급하고 사용량 제한을
  걸어라. 키를 SD에 두고 싶지 않으면 `config.toml`의 `api_key`를 비우고
  `ANTHROPIC_API_KEY` 환경변수를 쓴다.
- **시각이 틀리면 온라인 기능이 전부 막힌다.** TLS 인증서 검증이 실패하기
  때문이다. 앱이 먼저 확인해서 알려주지만, `date`로 미리 봐 두면 좋다.
