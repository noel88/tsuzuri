#!/bin/sh
# tsuzuri 단축어를 만든다. 기기에서 한 번만 실행하면 된다.
#
#   sh install.sh
#
# 그 뒤로는 어느 디렉터리에서든 `tsuzuri` 만 치면 실행된다.

set -e

here=$(cd "$(dirname "$0")" && pwd)
bin_dir=${BIN_DIR:-/usr/local/bin}
link="$bin_dir/tsuzuri"

if [ ! -x "$here/tsuzuri" ]; then
	echo "$here/tsuzuri 가 없거나 실행 권한이 없습니다." >&2
	exit 1
fi
chmod +x "$here"/*.sh "$here/tsuzuri" 2>/dev/null || true

mkdir -p "$bin_dir"

# run.sh를 부르는 작은 실행기를 둔다. 심볼릭 링크로 걸면 run.sh가 자기
# 위치를 찾지 못해 팩과 키를 놓친다.
cat > "$link" <<WRAPPER
#!/bin/sh
exec sh "$here/run.sh" "\$@"
WRAPPER
chmod +x "$link"

echo "설치했습니다: $link → $here/run.sh"
echo
case ":$PATH:" in
*":$bin_dir:"*)
	echo "이제 어디서든 tsuzuri 라고 치면 실행됩니다."
	;;
*)
	echo "주의: $bin_dir 이 PATH에 없습니다. ~/.profile 에 아래를 넣으세요:"
	echo "  export PATH=\"$bin_dir:\$PATH\""
	;;
esac
