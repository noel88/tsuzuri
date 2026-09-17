#!/usr/bin/env python3
"""Tsuzuri 화면 캡처 — 포트폴리오·README용.

실제 바이너리를 가상 터미널(pty)에서 돌리고, 입력을 넣은 뒤 그 순간 화면에
찍힌 출력을 포메라 DM250 해상도(1024x600)로 렌더링한다. 손으로 그린 목업이
아니라 프로그램 출력 그대로다.

    python3 tools/screens/shoot.py            # docs/screenshots/ 에 저장

필요한 것: Go, Pillow, 폰트 두 개(환경변수로 바꿀 수 있다)
    TSUZURI_FONT_MAIN  라틴·한글·기호  (기본: D2Coding)
    TSUZURI_FONT_JA    가나·한자       (기본: 히라기노 각고딕)

실기 사진이 아니다. 다만 칸 수는 실기에서 잰 값이다(2026-09-17, stty 36 128).
포메라 fbterm은 1024x600 화면에 128x36칸, 즉 8x16 픽셀 글꼴을 쓴다.
"""

import fcntl
import os
import select
import shutil
import struct
import subprocess
import sys
import tempfile
import termios
import time
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

# 실기 실측: stty 36 128 → 1024x600 화면에 128x36칸, 8x16 글꼴.
#
# 칸 너비는 글꼴 크기의 절반이어야 한다. 고정폭 CJK 글꼴은 라틴이 0.5em,
# 한글·가나·한자가 1em이라, 그래야 전각 문자가 칸 두 개를 빈틈 없이 채운다.
COLS, ROWS = 128, 36
CELL_W, CELL_H = 8, 16
SCREEN_W, SCREEN_H = 1024, 600
SCALE = 3  # 8x16 글꼴은 작아서 3배로 키운다.
MARGIN = 8  # 위아래 여백(픽셀, 1배 기준)

REPO = Path(__file__).resolve().parents[2]
OUT = REPO / "docs" / "screenshots"

FONT_MAIN = os.environ.get(
    "TSUZURI_FONT_MAIN",
    str(Path.home() / "Library/Fonts/D2Coding-Ver1.3.2-20180524.ttc"))
FONT_JA = os.environ.get(
    "TSUZURI_FONT_JA", "/System/Library/Fonts/ヒラギノ角ゴシック W4.ttc")

THEMES = {
    # fbterm 기본에 가까운 어두운 화면
    "dark": {"bg": (11, 11, 11), "fg": (216, 216, 216)},
    # 종이 느낌. 포메라 사진 옆에 두기 좋다
    "paper": {"bg": (236, 233, 224), "fg": (28, 28, 28)},
}

# internal/ui/width.go 와 같은 범위. 화면 격자 계산이 앱과 일치해야 한다.
WIDE = [
    (0x1100, 0x115F), (0x2E80, 0x303E), (0x3041, 0x33FF), (0x3400, 0x4DBF),
    (0x4E00, 0x9FFF), (0xA000, 0xA4CF), (0xAC00, 0xD7A3), (0xF900, 0xFAFF),
    (0xFE30, 0xFE6F), (0xFF00, 0xFF60), (0xFFE0, 0xFFE6),
    (0x20000, 0x2FFFD), (0x30000, 0x3FFFD),
]


def cell_width(ch):
    o = ord(ch)
    return 2 if any(a <= o <= b for a, b in WIDE) else 1


def is_japanese(ch):
    o = ord(ch)
    return (0x3000 <= o <= 0x30FF or 0x3400 <= o <= 0x4DBF
            or 0x4E00 <= o <= 0x9FFF or 0xFF00 <= o <= 0xFFEF)


# ---------------------------------------------------------------- 가상 터미널

class Screen:
    """제어 문자를 쓰지 않는 줄 단위 출력만 해석하면 된다."""

    def __init__(self):
        self.lines = []
        self.cur = ""
        self.col = 0

    def feed(self, text):
        for ch in text:
            if ch == "\r":
                continue
            if ch == "\n":
                self.lines.append(self.cur)
                self.cur, self.col = "", 0
                continue
            if ch == "\b":
                self.cur = self.cur[:-1]
                continue
            w = cell_width(ch)
            if self.col + w > COLS:
                self.lines.append(self.cur)
                self.cur, self.col = "", 0
            self.cur += ch
            self.col += w

    def current_page(self):
        """마지막 화면의 머리(╔ 배너나 ┌ 상자)부터 끝까지.

        캡처는 화면 지우기를 끄고(TSUZURI_NO_CLEAR=1) 받으므로 출력이
        이어진다. 여기서 마지막 한 화면만 잘라낸다. 실기에서는 앱이
        ANSI로 화면을 지우므로 보이는 것은 이 한 화면뿐이다.
        """
        all_lines = self.lines + [self.cur]
        start = 0
        for i in range(len(all_lines) - 1, -1, -1):
            s = all_lines[i].lstrip()
            if s.startswith("╔") or s.startswith("┌"):
                start = i
                break
        return all_lines[start:start + ROWS]


class Session:
    def __init__(self, binary, data_dir):
        self.master, slave = os.openpty()
        # 앱이 뜨기 전에 폭을 정해 둔다. 앱은 TIOCGWINSZ로 이 값을 읽는다.
        fcntl.ioctl(slave, termios.TIOCSWINSZ,
                    struct.pack("HHHH", ROWS, COLS, 0, 0))
        # 화면 지우기를 끄고 받는다. 앱은 실기에서 ANSI로 화면을 지우지만,
        # 여기서는 출력이 이어져야 화면 하나를 통째로 잘라낼 수 있다.
        # 결과는 같다 — 실기도 한 번에 한 화면만 보인다.
        env = dict(os.environ, TSUZURI_DATA=str(data_dir), TERM="linux",
                   TSUZURI_NO_CLEAR="1")
        env.pop("COLUMNS", None)
        self.proc = subprocess.Popen(
            [str(binary)], stdin=slave, stdout=slave, stderr=slave,
            env=env, cwd=data_dir, start_new_session=True)
        os.close(slave)
        self.screen = Screen()
        self.raw = ""
        self.mark = 0
        self._pending = b""

    def _pump(self, timeout):
        r, _, _ = select.select([self.master], [], [], timeout)
        if not r:
            return False
        try:
            chunk = os.read(self.master, 65536)
        except OSError:
            return False
        if not chunk:
            return False
        data = self._pending + chunk
        try:
            text = data.decode("utf-8")
            self._pending = b""
        except UnicodeDecodeError as e:
            text = data[:e.start].decode("utf-8")
            self._pending = data[e.start:]
        self.raw += text
        self.screen.feed(text)
        return True

    def expect(self, needle, timeout=30):
        deadline = time.time() + timeout
        while time.time() < deadline:
            idx = self.raw.find(needle, self.mark)
            if idx >= 0:
                self.mark = idx + len(needle)
                self._settle()
                return
            self._pump(0.2)
        tail = self.raw[-600:]
        raise TimeoutError(f"{needle!r} 가 {timeout}초 안에 나오지 않았다:\n{tail}")

    def _settle(self):
        # 프롬프트 뒤에 따라오는 출력을 마저 받는다.
        while self._pump(0.3):
            pass

    def type(self, text):
        """Enter 없이 친다. 터미널이 에코하므로 입력 중인 모습이 찍힌다."""
        os.write(self.master, text.encode("utf-8"))
        time.sleep(0.3)
        self._settle()

    def send(self, line):
        os.write(self.master, (line + "\n").encode("utf-8"))

    def close(self):
        try:
            self.proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.proc.kill()
        os.close(self.master)


# ---------------------------------------------------------------- 렌더링

BOX = {
    "─": ("light", "h"), "│": ("light", "v"),
    "┌": ("light", +1, +1), "┐": ("light", -1, +1),
    "└": ("light", +1, -1), "┘": ("light", -1, -1),
    "═": ("double", "h"), "║": ("double", "v"),
    "╔": ("double", +1, +1), "╗": ("double", -1, +1),
    "╚": ("double", +1, -1), "╝": ("double", -1, -1),
}


def draw_box(d, ch, x, y, cw, chh, fg, lw):
    """상자 문자는 직접 그린다. 폰트 글리프는 칸 끝까지 닿지 않아
    선이 끊겨 보이는데, 터미널은 이것을 이어서 그린다."""
    spec = BOX[ch]
    cx, cy = x + cw // 2, y + chh // 2
    right, bottom = x + cw, y + chh
    g = max(2, round(cw * 0.2))

    def hline(yy, x0, x1):
        d.line([(min(x0, x1), yy), (max(x0, x1), yy)], fill=fg, width=lw)

    def vline(xx, y0, y1):
        d.line([(xx, min(y0, y1)), (xx, max(y0, y1))], fill=fg, width=lw)

    style = spec[0]
    if spec[1] == "h":
        for off in ((0,) if style == "light" else (-g, g)):
            hline(cy + off, x, right)
        return
    if spec[1] == "v":
        for off in ((0,) if style == "light" else (-g, g)):
            vline(cx + off, y, bottom)
        return

    hx, vy = spec[1], spec[2]
    hedge = right if hx > 0 else x
    vedge = bottom if vy > 0 else y
    if style == "light":
        hline(cy, cx, hedge)
        vline(cx, cy, vedge)
        return
    for sign in (-1, +1):  # 바깥 선, 안쪽 선
        ox, oy = cx + sign * hx * g, cy + sign * vy * g
        hline(oy, ox, hedge)
        vline(ox, oy, vedge)


def render(page, path, theme):
    """한 화면을 그린다.

    실기 화면은 128x36칸인데 한 화면의 내용은 20줄 남짓이라 아래가 빈다.
    빈 줄을 걷어내고 쓴 만큼만 그린다.
    """
    s = SCALE
    cw, chh = CELL_W * s, CELL_H * s

    used = len(page)
    while used > 0 and page[used - 1].strip() == "":
        used -= 1
    used += 1  # 프롬프트 아래 한 줄은 커서 자리로 남긴다
    if used > ROWS:
        used = ROWS

    height = used * chh + 2 * MARGIN * s
    img = Image.new("RGB", (SCREEN_W * s, height), theme["bg"])
    d = ImageDraw.Draw(img)
    size = 2 * cw  # 1em = 칸 두 개
    main = ImageFont.truetype(FONT_MAIN, size)
    ja = ImageFont.truetype(FONT_JA, size)
    ox = (SCREEN_W * s - COLS * cw) // 2
    oy = MARGIN * s
    asc, desc = main.getmetrics()
    base_off = (chh - (asc + desc)) // 2 + asc
    lw = max(1, round(s * 1.2))

    for r, line in enumerate(page):
        col = 0
        for ch in line:
            w = cell_width(ch)
            if col + w > COLS:
                break
            x, y = ox + col * cw, oy + r * chh
            if ch in BOX:
                draw_box(d, ch, x, y, cw, chh, theme["fg"], lw)
            elif ch != " ":
                font = ja if is_japanese(ch) else main
                d.text((x + w * cw / 2, y + base_off), ch,
                       font=font, fill=theme["fg"], anchor="ms")
            col += w
    img.save(path, optimize=True)


# ---------------------------------------------------------------- 시나리오

SAMPLE_FEEDBACK = {
    # 화면을 보여주기 위한 예시 첨삭이다. 실제 API 응답이 아니다.
    "attempt_id": "demo-a1",
    "at": "2026-09-15T10:00:00+09:00",
    "corrected": "昨日初めて行ったカフェは思ったより静かで、ずっと座っていた。",
    "notes": [
        {"span": "静かくて", "why": "な형용사는 「静かで」로 활용한다", "level": "error"},
        {"span": "ました", "why": "문제는 보통체다. 「いた」로 맞춘다", "level": "error"},
        {"span": "ずっと", "why": "틀리지 않지만 「長く」가 시간의 길이에 더 가깝다", "level": "nuance"},
    ],
    "overall": "활용 하나와 문체만 고치면 뜻은 정확히 전달됩니다.",
}


ANSWERS = {
    "demo-a1": "昨日初めて行ったカフェは静かくて、ずっと座ってました。",
    "demo-a2": "雨が降りそうで傘を持ってきました。",
    "demo-a3": "昨日初めて行ったカフェが思ったより静かで、長く座っていました。",
    "demo-a4": "雨が降りそうなのに傘を持ってきました。",
    "demo-a5": "昨日初めて行ったカフェが思ったより静かで、長く座っていた。",
    "demo-a6": "雨が降るので傘を持ってきました。",
    "demo-a7": "昨日行ったカフェは静かでした。",
    "demo-a8": "어제 처음 간 카페가 생각보다 조용해서 오래 앉아 있었다.",
    "demo-a9": "어제 처음 간 카페가 생각보다 조용해서 한참 있었다.",
}


def seed(data):
    """화면을 보여주기 위한 예시 데이터. 실제 사용 기록이 아니다.

    진도 화면은 며칠치 기록이 쌓여야 보여줄 것이 생긴다. 날짜는 캡처를
    뽑는 날 기준으로 만든다 — 고정 날짜로 박아 두면 몇 달 뒤에 찍은 캡처의
    「연속 3일」이 3년 전 날짜 위에 앉는다.
    """
    import json
    from datetime import datetime, timedelta

    packs = data / "packs"
    packs.mkdir(parents=True)
    for name in ("sample-ko2ja.jsonl", "sample-ja2ko.jsonl"):
        shutil.copy(REPO / "testdata" / "packs" / name, packs / name)

    now = datetime.now().astimezone()

    def when(days, hour=21):
        d = (now - timedelta(days=days)).replace(hour=hour, minute=40, second=0,
                                                 microsecond=0)
        return d.isoformat()

    # (id, 문제, 며칠 전, 빠뜨린 표현)
    rows = [
        ("demo-a7", "p001", 7, ["初めて", "〜ていた"]),
        ("demo-a6", "p002", 6, ["〜そう", "ので"]),
        ("demo-a8", "s-ja001", 3, ["생각보다"]),
        ("demo-a1", "p001", 3, ["初めて"]),
        ("demo-a2", "p002", 3, ["〜そう", "ので"]),
        ("demo-a3", "p001", 2, ["〜ていた"]),
        ("demo-a9", "s-ja001", 1, ["앉아 있"]),
        ("demo-a4", "p002", 1, ["〜そう"]),
        ("demo-a5", "p001", 0, []),
    ]
    with open(data / "attempts.jsonl", "w") as f:
        for aid, pid, days, missing in rows:
            answer = ANSWERS.get(aid, "…")
            f.write(json.dumps({
                "id": aid, "pack_id": pid, "at": when(days),
                "answer": answer,
                "analysis": {"missing": missing},
            }, ensure_ascii=False) + "\n")

    # 첨삭이 도착한 것만 복습 판정에 쓰인다.
    #
    # demo-a1 은 첨삭 화면에 쓸 예시다. 나머지는 진도 화면이 보여줄 상태를
    # 만든다 — p002 는 다시 틀려서 지금 풀 차례이고, p001 은 맞혀서 하루 뒤로
    # 물러났고, s-ja001 은 다시 답했지만 첨삭이 아직 안 왔다.
    graded = [
        SAMPLE_FEEDBACK,
        {"attempt_id": "demo-a2", "at": when(3, 22), "corrected": "",
         "notes": [{"span": "降りそうで", "why": "「〜そうなので」가 자연스럽다",
                    "level": "error"}],
         "overall": "이유를 잇는 꼴만 손보면 됩니다."},
        {"attempt_id": "demo-a4", "at": when(1, 22), "corrected": "",
         "notes": [{"span": "そうなのに", "why": "역접이 아니라 이유다",
                    "level": "error"}],
         "overall": "같은 자리에서 한 번 더 걸렸습니다."},
        {"attempt_id": "demo-a8", "at": when(3, 22), "corrected": "",
         "notes": [{"span": "조용해서", "why": "「静かで」는 나열이다",
                    "level": "error"}],
         "overall": "연결어미 하나만 고치면 됩니다."},
        {"attempt_id": "demo-a5", "at": when(0, 22), "corrected": "",
         "notes": [{"span": "長く", "why": "틀리지 않지만 「ずっと」도 쓴다",
                    "level": "nuance"}],
         "overall": "이번에는 짚을 것이 없습니다."},
    ]
    with open(data / "feedback.jsonl", "w") as f:
        for fb in graded:
            f.write(json.dumps(fb, ensure_ascii=False) + "\n")


def main():
    for font in (FONT_MAIN, FONT_JA):
        if not Path(font).exists():
            sys.exit(f"폰트가 없다: {font}\n환경변수 TSUZURI_FONT_MAIN / TSUZURI_FONT_JA 로 지정하라")

    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        binary = tmp / "tsuzuri"
        subprocess.run(["go", "build", "-o", str(binary), "."], cwd=REPO, check=True)
        data = tmp / "data"
        seed(data)

        pages = {}
        s = Session(binary, data)
        try:
            s.expect("선택 >>")
            pages["01-menu"] = s.screen.current_page()

            s.send("41")
            s.expect("답 >>")
            s.type("昨日初めて行ったカフェは静かくて、ずっと座ってました。")
            pages["02-problem"] = s.screen.current_page()

            s.send("")
            s.expect("선택 >>")
            pages["03-result"] = s.screen.current_page()

            s.send("r")
            s.expect("답 >>")
            s.send("昨日初めて行ったカフェが思ったより静かで、長く座っていた。")
            s.expect("선택 >>")
            pages["04-result-quiet"] = s.screen.current_page()

            s.send("m")
            s.expect("선택 >>")
            s.send("4")  # 첨삭 보기
            s.expect("선택 >>")
            pages["05-review"] = s.screen.current_page()

            s.send("m")
            s.expect("선택 >>")
            s.send("5")  # 진도
            s.expect("선택 >>")
            pages["06-progress"] = s.screen.current_page()

            s.send("m")
            s.expect("선택 >>")
            s.send("7")  # 설정
            s.expect("선택 >>")
            pages["07-setup"] = s.screen.current_page()

            s.send("")
            s.expect("선택 >>")
            s.send("x")
        finally:
            s.close()

    for theme_name, theme in THEMES.items():
        out = OUT / theme_name
        out.mkdir(parents=True, exist_ok=True)
        for name, page in pages.items():
            path = out / f"{name}.png"
            render(page, path, theme)
            print(path.relative_to(REPO))


if __name__ == "__main__":
    main()
