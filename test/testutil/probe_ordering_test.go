package testutil

import "testing"

func TestCanonicalPageKeyForProbeName(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		out  string
	}{
		{name: "page95", in: "009_p95_sfrm1095", out: "page95"},
		{name: "page109", in: "009_p109_sfrm1095_top5", out: "page109"},
		{name: "unknown", in: "unexpected", out: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanonicalPageKeyForProbeName(tc.in); got != tc.out {
				t.Fatalf("CanonicalPageKeyForProbeName(%q) = %q, want %q", tc.in, got, tc.out)
			}
		})
	}
}

func TestLargerProbeName(t *testing.T) {
	if got := LargerProbeName("009_p95_sfrm1095", 1.0, "009_p109_sfrm1095", 2.0); got != "009_p109_sfrm1095" {
		t.Fatalf("LargerProbeName() = %q, want %q", got, "009_p109_sfrm1095")
	}

	if got := LargerProbeName("009_p95_sfrm1095", 3.0, "009_p109_sfrm1095", 2.0); got != "009_p95_sfrm1095" {
		t.Fatalf("LargerProbeName() = %q, want %q", got, "009_p95_sfrm1095")
	}
}

func TestLargerProbeCanonicalPageKey(t *testing.T) {
	if got := LargerProbeCanonicalPageKey("009_p95_sfrm1095", 1.0, "009_p109_sfrm1095", 2.0); got != "page109" {
		t.Fatalf("LargerProbeCanonicalPageKey() = %q, want %q", got, "page109")
	}

	if got := LargerProbeCanonicalPageKey("009_p95_sfrm1095_top6", 3.0, "009_p109_sfrm1095_top5", 2.0); got != "page95" {
		t.Fatalf("LargerProbeCanonicalPageKey() = %q, want %q", got, "page95")
	}
}

func TestSmallerProbeName(t *testing.T) {
	if got := SmallerProbeName("009_p95_sfrm1095", 3.0, "009_p109_sfrm1095", 2.0); got != "009_p109_sfrm1095" {
		t.Fatalf("SmallerProbeName() = %q, want %q", got, "009_p109_sfrm1095")
	}

	if got := SmallerProbeName("009_p95_sfrm1095", 1.0, "009_p109_sfrm1095", 2.0); got != "009_p95_sfrm1095" {
		t.Fatalf("SmallerProbeName() = %q, want %q", got, "009_p95_sfrm1095")
	}
}

func TestSmallerProbeCanonicalPageKey(t *testing.T) {
	if got := SmallerProbeCanonicalPageKey("009_p95_sfrm1095", 3.0, "009_p109_sfrm1095", 2.0); got != "page109" {
		t.Fatalf("SmallerProbeCanonicalPageKey() = %q, want %q", got, "page109")
	}

	if got := SmallerProbeCanonicalPageKey("009_p95_sfrm1095_top6", 1.0, "009_p109_sfrm1095_top5", 2.0); got != "page95" {
		t.Fatalf("SmallerProbeCanonicalPageKey() = %q, want %q", got, "page95")
	}
}

func TestSharedCanonicalPageKey(t *testing.T) {
	testCases := []struct {
		name string
		keys []string
		want string
	}{
		{name: "same", keys: []string{"page109", "page109", "page109"}, want: "page109"},
		{name: "mismatch", keys: []string{"page95", "page109"}, want: ""},
		{name: "empty", keys: []string{"page109", ""}, want: ""},
		{name: "none", keys: nil, want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SharedCanonicalPageKey(tc.keys...); got != tc.want {
				t.Fatalf("SharedCanonicalPageKey(%v) = %q, want %q", tc.keys, got, tc.want)
			}
		})
	}
}

func TestProbeOrderingAlignment_SharedCanonicalKey(t *testing.T) {
	same := NewProbeOrderingAlignment("page109", "page109")
	if got := same.SharedCanonicalKey(); got != "page109" {
		t.Fatalf("SharedCanonicalKey() = %q, want %q", got, "page109")
	}

	mismatch := NewProbeOrderingAlignment("page95", "page109")
	if got := mismatch.SharedCanonicalKey(); got != "" {
		t.Fatalf("SharedCanonicalKey() = %q, want empty", got)
	}
}
