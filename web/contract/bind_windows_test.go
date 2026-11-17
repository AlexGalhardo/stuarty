//go:build windows

package contract

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareChild(*exec.Cmd) {}

// bindChild puts the started child in a job object that kills it when the last handle to the job
// closes. The handle belongs to the test process, so a timeout or a kill of `go test` takes the
// server down with it instead of leaving an orphan (VV-188).
func bindChild(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create the job object: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}

	if _, err := windows.SetInformationJobObject(
		job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)

		return fmt.Errorf("set the job limits: %w", err)
	}

	pid := uint32(cmd.Process.Pid)

	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		_ = windows.CloseHandle(job)

		return fmt.Errorf("open the child: %w", err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)

		return fmt.Errorf("assign the child to the job: %w", err)
	}

	// The job handle is deliberately never closed: it lives as long as the test process.
	return nil
}

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()

	ev, err := windows.WaitForSingleObject(h, 0)

	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}
