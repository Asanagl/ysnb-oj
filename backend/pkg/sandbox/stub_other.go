//go:build !linux

// Package sandbox compiles on non-Linux platforms as a stub so the rest of
// the codebase (API server, judge service) type-checks everywhere; actually
// jailing processes is only implemented for Linux.
package sandbox

import (
	"errors"
	"os"
)

// StubSandbox refuses to run: use the real implementation on Linux.
type Sandbox struct {
	CgroupBase string
	HidePaths  []string
}

func (s *Sandbox) Preflight() error {
	return errors.New("sandbox requires Linux (cgroup v2 + namespaces)")
}

func (s *Sandbox) Run(spec *Spec, files *Files) (*Result, error) {
	return nil, errors.New("sandbox requires Linux")
}

func (s *Sandbox) RunPair(userSpec, interSpec *Spec, stderr *os.File) (*Result, *Result, error) {
	return nil, nil, errors.New("sandbox requires Linux")
}

// Stage2Main is never reached on non-Linux; the daemon main refuses earlier.
func Stage2Main() {
	os.Exit(1)
}
