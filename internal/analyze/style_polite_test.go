package analyze

import "testing"

func TestIsKoreanPoliteReadsJongseong(t *testing.T) {
	// 「갑시다」의 「ㅂ」은 앞 음절에 합쳐져 있어 글자로는 못 찾는다.
	// 못 잡으면 정중체로 제대로 답한 학습자가 문체 어긋남 지적을 받는다.
	// 「그렇다니까」 같은 반말을 정중체로 잡아서도 안 된다.
	cases := map[string]bool{
		"갑시다": true, "합시다": true, "입니다": true, "갔습니까": true,
		"하세요": true, "주십시오": true, "예요": true,
		"그렇다니까": false, "하니까": false, "간다": false, "먹었다": false, "이다": false,
	}
	for s, want := range cases {
		if got := isKoreanPolite(s); got != want {
			t.Errorf("isKoreanPolite(%q) = %v, 기대 %v", s, got, want)
		}
	}
}
