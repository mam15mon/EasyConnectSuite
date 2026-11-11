//go:build !linux || android

package topsec

// RunDeviceCodeHelper handles only the Linux privileged identity entry point.
func RunDeviceCodeHelper(args []string) (bool, error) { return false, nil }
