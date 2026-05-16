// Package psi provides version-detection helpers for Pressure Stall
// Information support. PSI requires Kubernetes 1.34+ for Beta access (subject
// to the kubelet zero-emission bug, kubernetes/kubernetes#136333) and 1.36+
// for GA. The Linux kernel must be at least 4.20.
package psi

import (
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/version"
)

const gaMajor = 1
const gaMinor = 36

const kernelMajor = 4
const kernelMinor = 20

// ServerVersionOK reports whether the cluster server version is at or above
// the PSI GA release (Kubernetes 1.36). Returns false on parse failure
// (conservative default for unrecognized version strings).
func ServerVersionOK(v *version.Info) bool {
	if v == nil {
		return false
	}
	major, err := strconv.Atoi(strings.TrimSuffix(v.Major, "+"))
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(strings.TrimSuffix(v.Minor, "+"))
	if err != nil {
		return false
	}
	if major > gaMajor {
		return true
	}
	if major == gaMajor && minor >= gaMinor {
		return true
	}
	return false
}

// KernelVersionOK reports whether the node's Linux kernel version supports
// PSI (>= 4.20). The kernelVersion string is typically formatted as
// "<major>.<minor>.<patch>-<flavor>" (e.g. "5.15.0-72-generic"). Returns false
// on parse failure (conservative default for unrecognized version strings).
func KernelVersionOK(kernelVersion string) bool {
	if kernelVersion == "" {
		return false
	}
	// Strip everything after the first '-' (distro flavor).
	if i := strings.IndexByte(kernelVersion, '-'); i > 0 {
		kernelVersion = kernelVersion[:i]
	}
	parts := strings.SplitN(kernelVersion, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	if major > kernelMajor {
		return true
	}
	if major == kernelMajor && minor >= kernelMinor {
		return true
	}
	return false
}
