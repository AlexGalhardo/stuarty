//go:build !linux

package gioapp

// Other systems have no device list here yet: playback stays picture-only and
// says so. The video clock does not depend on the device.
func linuxAudioDevices() [][2]string { return nil }
