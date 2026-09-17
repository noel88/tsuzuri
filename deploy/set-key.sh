#!/bin/sh
# API 키를 입력받아 저장한다. 입력하는 동안 화면에 보이지 않는다.
#
#   sh set-key.sh              ~/.config/tsuzuri/key 에 저장
#   sh set-key.sh /mnt/sd/...  다른 경로에 저장
#
# 키를 명령줄 인자로 주지 않는다. 인자로 주면 셸 기록(~/.sh_history 등)과
# 프로세스 목록에 그대로 남는다.

set -e

key_file=${1:-"$HOME/.config/tsuzuri/key"}
dir=$(dirname "$key_file")

mkdir -p "$dir"

printf 'Anthropic API 키를 붙여넣고 Enter (화면에 보이지 않습니다): '

# 입력하는 동안 에코를 끈다. 끝나면 어떤 경우에도 되돌린다.
if [ -t 0 ] && command -v stty >/dev/null 2>&1; then
	saved=$(stty -g)
	trap 'stty "$saved" 2>/dev/null; echo' EXIT INT TERM
	stty -echo
	# 개행 없이 끝나는 입력(붙여넣기)도 받아야 하므로 실패를 무시한다.
	read -r key || true
	stty "$saved"
	trap - EXIT INT TERM
	echo
else
	# 터미널이 아니면(파이프 등) 에코를 끌 필요가 없다.
	# pbpaste처럼 개행 없이 끝나는 입력도 받아야 하므로 실패를 무시한다.
	read -r key || true
fi

# 앞뒤 공백과 개행을 떼어낸다. 붙여넣기에는 잘 섞인다.
key=$(printf '%s' "$key" | tr -d ' \t\r\n')

if [ -z "$key" ]; then
	echo "입력이 비어 있습니다. 저장하지 않았습니다." >&2
	exit 1
fi

case "$key" in
sk-ant-*) ;;
*)
	echo "sk-ant- 로 시작하지 않습니다. 잘못 붙여넣었는지 확인하세요." >&2
	echo "저장하지 않았습니다." >&2
	exit 1
	;;
esac

# 임시 파일에 쓰고 옮긴다. 쓰는 도중에 전원이 끊겨도 반쯤 남지 않는다.
tmp="$key_file.tmp"
umask 077
printf '%s\n' "$key" > "$tmp"
chmod 600 "$tmp" 2>/dev/null || true
mv "$tmp" "$key_file"

# 확인은 마스킹해서만 보여준다.
head=$(printf '%.10s' "$key")
len=$(printf '%s' "$key" | wc -c | tr -d ' ')
echo "저장했습니다: $key_file"
echo "  키: ${head}... (${len}자)"
ls -l "$key_file" | awk '{print "  권한:", $1}'
echo
echo "이제 sh run.sh 로 실행하면 이 키를 씁니다."
