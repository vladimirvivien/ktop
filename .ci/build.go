package main

import (
	"fmt"

	"github.com/vladimirvivien/gexe"
)

var (
	PkgRoot = "github.com/vladimirvivien/ktop"
	Version = fmt.Sprintf("%s-unreleased", gexe.Run("git rev-parse --abbrev-ref HEAD"))
	GitSHA  = gexe.Run("git rev-parse HEAD")
)

// main use this for local build
// go run build.go
func main() {
	arches := []string{"amd64", "arm64"}
	oses := []string{"darwin", "linux"}

	ldflags := fmt.Sprintf(
		`"-X %s/buildinfo.Version=%s -X %s/buildinfo.GitSHA=%s"`,
		PkgRoot, Version, PkgRoot, GitSHA,
	)

	for _, arch := range arches {
		for _, os := range oses {
			binary := fmt.Sprintf(".build/%s/%s/ktop", arch, os)
			gobuild(arch, os, ldflags, binary)
		}
	}
}

func gobuild(arch, os, ldflags, binary string) {
	gexe.SetVar("arch", arch)
	gexe.SetVar("os", os)
	gexe.SetVar("ldflags", ldflags)
	gexe.SetVar("binary", binary)
	result := gexe.Envs("CGO_ENABLED=0 GOOS=$os GOARCH=$arch").Run("go build -o $binary -ldflags $ldflags .")
	if result != "" {
		fmt.Printf("Build for %s/%s failed: %s\n", arch, os, result)
		return
	}
	fmt.Printf("Build %s/%s OK: %s\n", arch, os, binary)
}
