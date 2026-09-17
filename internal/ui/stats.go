package ui

import (
	"fmt"
	"strconv"
	"time"

	"github.com/noel88/tsuzuri/internal/progress"
)

// RenderStats는 진도 화면이다.
//
// 점수는 없다 (스펙 §5.4). 여기 있는 것은 전부 「얼마나 했는지」와 「무엇이
// 남았는지」이지, 「얼마나 잘했는지」가 아니다. 자주 빠뜨린 표현도 못한다는
// 뜻이 아니라 다음에 볼 곳을 가리키는 것이다.
func RenderStats(s progress.Stats, cards []progress.Card, labels map[string]string, st Status, termW int) string {
	w := BoxWidth(termW)
	// 수치 칸은 넓혀도 읽기 좋아지지 않는다. 화면을 채우면 숫자만 저 멀리
	// 오른쪽 끝으로 가서 어느 줄의 값인지 눈으로 따라가야 한다.
	inner := w - 4
	if inner > 56 {
		inner = 56
	}

	var lines []string
	lines = append(lines, Frame("진 도", "", w)...)
	lines = append(lines, "")

	lines = append(lines, Row("연속", plural(s.Streak, "일"), inner))
	lines = append(lines, Row("푼 날", plural(s.Days, "일"), inner))
	lines = append(lines, Row("제출한 답안", plural(s.Total, "건"), inner))
	lines = append(lines, Row("손댄 문제", plural(s.Problems, "개"), inner))
	lines = append(lines, Row("받은 첨삭", plural(s.Feedback, "건"), inner))

	lines = append(lines, "")
	lines = append(lines, "  "+Divider(inner))
	lines = append(lines, "")
	lines = append(lines, Row("복습할 문제", plural(s.Due, "개"), inner))
	lines = append(lines, Row("첨삭 기다리는 중", plural(s.Waiting, "개"), inner))
	lines = append(lines, Row("졸업", plural(s.Graduated, "개"), inner))

	if len(s.TopMissing) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+Divider(inner))
		lines = append(lines, "")
		lines = append(lines, "  자주 빠뜨린 표현")
		for _, m := range s.TopMissing {
			lines = append(lines, Row("  "+m.Text, plural(m.N, "번"), inner))
		}
	}

	if next := nextDue(cards); len(next) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+Divider(inner))
		lines = append(lines, "")
		lines = append(lines, "  다음 복습")
		for _, c := range next {
			label := labels[c.ProblemID]
			if label == "" {
				label = c.ProblemID
			}
			lines = append(lines, Row("  "+label, whenLabel(c.Due, st.Now), inner))
		}
	}

	lines = append(lines, "")
	lines = append(lines, CommandBarWith(
		"진도",
		"주요명령(돌아가기 ⏎)  메뉴(M)  종료(X)",
		"선택 >>", w)...)

	return Page(lines, termW, w)
}

// nextDue는 아직 차례가 아닌 것 중 가까운 것부터 몇 개를 고른다.
func nextDue(cards []progress.Card) []progress.Card {
	var out []progress.Card
	for _, c := range cards {
		if c.State == progress.StateScheduled {
			out = append(out, c)
			if len(out) == 3 {
				break
			}
		}
	}
	return out
}

// whenLabel은 남은 시간을 사람이 읽는 말로 바꾼다.
//
// 「9월 23일 14시」보다 「6일 뒤」가 쓸모 있다. 언제 다시 켤지 모르는
// 기기에서 정확한 시각은 알 이유가 없고, 얼마나 남았는지만 알면 된다.
func whenLabel(due time.Time, now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	d := due.Sub(now)
	switch {
	case d <= 0:
		return "지금"
	case d < time.Hour:
		return "잠시 뒤"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "시간 뒤"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "일 뒤"
	}
}

func plural(n int, unit string) string {
	return fmt.Sprintf("%d%s", n, unit)
}
