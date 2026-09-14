#!/bin/sh
# M0 실기 검증 — 포메라 DM250의 Debian 모드에서 실행한다.
#
#   sh m0-check.sh
#
# 스펙 §9의 R1~R6을 확인한다. 결과를 그대로 옮겨 적으면 된다.

echo "=============================================="
echo " Tsuzuri M0 실기 검증"
echo "=============================================="
echo

echo "--- 기본 정보 ---"
uname -m
echo "CPU: $(grep -m1 'model name\|Processor' /proc/cpuinfo 2>/dev/null | cut -d: -f2- | sed 's/^ //')"
echo "코어: $(grep -c ^processor /proc/cpuinfo 2>/dev/null)"
echo "클럭: $(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq 2>/dev/null || echo '?') kHz"
echo "메모리:"
free -m 2>/dev/null | head -2
echo

echo "--- R6: 글리프 (두부 □ 로 보이면 실패) ---"
echo "  박스: ┌ ─ ┐ │ └ ┘ ╔ ═ ╗ ║ ╚ ╝"
echo "  표식: ✓ ✗ ⚠ · ↔ ⏎"
echo "  한글: 가나다  일본어: 日本語かな  한자: 綴"
echo
echo "  위 줄들이 제대로 보입니까? (두부가 있으면 ASCII 폴백 필요)"
echo

echo "--- 터미널 폭 ---"
echo "  stty: $(stty size 2>/dev/null || echo '알 수 없음')"
echo "  tput cols: $(tput cols 2>/dev/null || echo '알 수 없음')"
echo "  0123456789 눈금 (한 줄이 몇 칸인지 확인):"
printf '  '
i=0
while [ $i -lt 12 ]; do printf '1234567890'; i=$((i+1)); done
echo
echo

echo "--- R5: 시각과 TLS ---"
date
if command -v curl >/dev/null 2>&1; then
	code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 \
		https://api.anthropic.com/v1/models 2>&1)
	echo "  api.anthropic.com → $code   (401이면 성공: TLS는 뚫렸고 인증만 없음)"
else
	echo "  curl이 없습니다. apt install curl 후 다시 확인하세요."
fi
echo

echo "--- R1: 사전 로딩 시간 ---"
if [ ! -x ./tsuzuri ]; then
	echo "  ./tsuzuri 가 없습니다. 바이너리를 이 디렉터리에 두세요."
	exit 1
fi
ls -lh ./tsuzuri | awk '{print "  바이너리 크기:", $5}'
echo
echo "  ko2ja(일본어 사전, 맥 기준 88MB) 로딩을 잽니다..."
echo "  메뉴가 뜨면 41 입력 → 사전 로딩 후 문제가 뜨면 x 로 종료하세요."
echo
time sh -c 'printf "41\nx\n" | ./tsuzuri' >/dev/null 2>&1
echo
echo "  ja2ko(한국어 사전, 맥 기준 211MB) 로딩을 잽니다..."
echo "  ja2ko 팩이 있어야 합니다. 없으면 건너뜁니다."
time sh -c 'printf "42\nx\n" | ./tsuzuri' >/dev/null 2>&1
echo

echo "--- R2: 한글 입력 (손으로 확인) ---"
echo "  1) fbterm 콘솔에서 아래를 실행하고 한글을 쳐 보세요:"
echo "       cat > /tmp/ko-test.txt"
echo "     (Ctrl+D로 종료, cat /tmp/ko-test.txt 로 확인)"
echo "  2) 안 되면 입력기 설치를 시도하세요:"
echo "       sudo apt install ibus-hangul   또는   nabi"
echo "  3) 콘솔에서 안 되면 X11에서 시도: startx 후 같은 확인"
echo
echo "  ja2ko 방향(한국어로 작문)이 여기에 달려 있습니다."
echo

echo "=============================================="
echo " 기록할 것: 사전 로딩 시간, 메모리(free -m), 터미널 폭,"
echo "            글리프 정상 여부, curl 응답 코드, 한글 입력 가부"
echo "=============================================="
