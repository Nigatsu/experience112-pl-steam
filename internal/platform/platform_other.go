//go:build !windows

package platform

import "errors"

var errUnsupported = errors.New("platform: not supported on this OS")

// InitConsole is a no-op outside Windows.
func InitConsole() {}

// IsWine reports false outside Windows.
func IsWine() bool { return false }

// DriveRoots returns nil outside Windows.
func DriveRoots() []string { return nil }

// SteamRoots returns nil outside Windows.
func SteamRoots() []string { return nil }

// UserDword reports no value outside Windows.
func UserDword(path, name string) (uint32, bool) { return 0, false }

// SetUserDword is unsupported outside Windows.
func SetUserDword(path, name string, v uint32) error { return errUnsupported }

// DeleteUserValue is unsupported outside Windows.
func DeleteUserValue(path, name string) error { return errUnsupported }

// PrimaryGPU reports an unknown adapter outside Windows.
func PrimaryGPU() (GPU, bool) { return GPU{}, false }
