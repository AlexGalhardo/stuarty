//go:build linux

package gioapp

// linuxAudioDevices is the order the preview tries. Pulse is what a desktop
// session usually has; ALSA and OSS cover a machine without it.
func linuxAudioDevices() [][2]string {
	return [][2]string{{"pulse", "default"}, {"alsa", "default"}, {"oss", "/dev/dsp"}}
}
