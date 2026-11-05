package psi

import (
	"testing"

	"k8s.io/apimachinery/pkg/version"
)

func TestServerVersionOK(t *testing.T) {
	cases := []struct {
		name  string
		major string
		minor string
		want  bool
	}{
		{"GA 1.36", "1", "36", true},
		{"future 1.37", "1", "37", true},
		{"future 2.0", "2", "0", true},
		{"beta 1.35", "1", "35", false},
		{"beta 1.34", "1", "34", false},
		{"pre-1.34 1.32", "1", "32", false},
		{"managed suffix 1.36+", "1", "36+", true},
		{"managed suffix 1.35+", "1", "35+", false},
		{"empty", "", "", false},
		{"unparseable", "v1", "x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &version.Info{Major: tc.major, Minor: tc.minor}
			if got := ServerVersionOK(v); got != tc.want {
				t.Errorf("ServerVersionOK(%q.%q) = %v, want %v", tc.major, tc.minor, got, tc.want)
			}
		})
	}
}

func TestServerVersionOK_NilInfo(t *testing.T) {
	if ServerVersionOK(nil) {
		t.Errorf("ServerVersionOK(nil) = true, want false")
	}
}

func TestKernelVersionOK(t *testing.T) {
	cases := []struct {
		kernel string
		want   bool
	}{
		{"5.15.0-72-generic", true},
		{"5.4.0-1100-aws", true},
		{"6.1.0-13-cloud-amd64", true},
		{"7.0.0-14-generic", true},
		{"4.20.0", true},
		{"4.20", true},
		{"4.19.0-15-generic", false},
		{"4.18.0-477.el8", false},
		{"3.10.0-1160.el7", false},
		{"", false},
		{"not-a-version", false},
		{"4", false}, // single number, no minor
	}
	for _, tc := range cases {
		t.Run(tc.kernel, func(t *testing.T) {
			if got := KernelVersionOK(tc.kernel); got != tc.want {
				t.Errorf("KernelVersionOK(%q) = %v, want %v", tc.kernel, got, tc.want)
			}
		})
	}
}
