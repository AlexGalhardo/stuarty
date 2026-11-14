package workspace

import "golang.org/x/sys/windows"

// FreeBytes is the space the calling user may still write on the volume of dir
// (SDD-007 section 9.2 item 4).
func FreeBytes(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}

	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}

	return free, nil
}
