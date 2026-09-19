//go:build !linux

package sysinfo

import "errors"

func statfs(string) (uint64, uint64, uint64, error) {
	return 0, 0, 0, errors.New("statfs is supported only on linux")
}
