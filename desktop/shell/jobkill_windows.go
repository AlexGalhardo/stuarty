//go:build windows

package shell

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// bindToShell puts the child in a job object that kills it when the last handle to the job
// closes. The handle belongs to this process, so a crash of the shell ends the server too
// instead of leaving an orphan that keeps the workspace lock (VV-006). The returned function
// closes the handle early; calling it kills the child.
func bindToShell(cmd *exec.Cmd) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create the job object: %w", err)
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

		return nil, fmt.Errorf("set the job limits: %w", err)
	}

	pid := uint32(cmd.Process.Pid) //nolint:gosec // a Windows pid is a 32-bit DWORD

	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		_ = windows.CloseHandle(job)

		return nil, fmt.Errorf("open the child: %w", err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)

		return nil, fmt.Errorf("assign the child to the job: %w", err)
	}

	return func() { _ = windows.CloseHandle(job) }, nil
}
