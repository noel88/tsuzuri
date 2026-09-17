#!/bin/sh
# Tsuzuri 실행기.
#
#   sh run.sh
#
# API 키를 SD카드가 아니라 기기 홈에 두고 환경변수로 넘긴다.
# SD카드는 vfat이라 권한을 좁힐 수 없고, 카드를 잃으면 키가 그대로 노출된다.
#
# 키를 처음 넣을 때(기기에서 직접, 한 번만):
#
#   sh set-key.sh

set -e

here=$(dirname "$0")
cd "$here"

key_file="$HOME/.config/tsuzuri/key"
if [ -z "$ANTHROPIC_API_KEY" ] && [ -f "$key_file" ]; then
	ANTHROPIC_API_KEY=$(tr -d ' \t\r\n' < "$key_file")
	export ANTHROPIC_API_KEY
fi

if [ -z "$ANTHROPIC_API_KEY" ]; then
	echo "API 키가 없습니다. 온라인 기능(5·6번)은 쓸 수 없습니다."
	echo "넣으려면: sh set-key.sh"
	echo
fi

: "${TSUZURI_DATA:=$PWD}"
export TSUZURI_DATA

exec ./tsuzuri "$@"
