//go:build !linux

package main

import (
	"errors"

	"github.com/ysnb/oj/pkg/sandbox"
)

func runEchoTest(sb *sandbox.Sandbox, workRoot string) error {
	return errors.New("selftest requires Linux")
}

func runTimeoutTest(sb *sandbox.Sandbox, workRoot string) error {
	return errors.New("selftest requires Linux")
}
