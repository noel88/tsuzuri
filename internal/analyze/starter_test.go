package analyze

import (
	"testing"

	"github.com/noel88/tsuzuri/internal/pack"
)

// 기기에 같이 넣는 시작 팩은 모범답안을 그대로 넣었을 때 조용해야 한다.
func TestStarterPacksAreQuiet(t *testing.T) {
	for _, dir := range []pack.Direction{pack.KoToJa, pack.JaToKo} {
		az, err := New(dir)
		if err != nil {
			t.Fatal(err)
		}
		ps, skipped, err := pack.LoadDir("../../deploy/packs", dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(skipped) != 0 {
			t.Fatalf("건너뛴 팩이 있다: %+v", skipped)
		}
		if len(ps) != 5 {
			t.Fatalf("%s 문항 = %d개, 기대 5개", dir, len(ps))
		}
		for _, p := range ps {
			got := az.Analyze(p, p.Reference)
			if got.StyleMismatch {
				t.Errorf("%s: 모범답안에 문체 경고 (요구 %q / 판정 %q)", p.ID, p.Style, got.DetectedStyle)
			}
			if len(got.Missing) > 0 {
				t.Errorf("%s: 모범답안인데 빠진 표현: %v", p.ID, got.Missing)
			}
			if len(got.Flags) > 0 {
				t.Errorf("%s: 모범답안에 지적: %+v", p.ID, got.Flags)
			}
		}
	}
}
