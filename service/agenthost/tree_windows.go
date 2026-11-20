//go:build windows

package agenthost

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// treeKiller ends every process of a session.
type treeKiller interface {
	Kill()
	Close()
}

// prepare starts the child without a console window of its own.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// jobKiller is a job object that kills every member when it is terminated or when the last
// handle to it closes, which includes the death of Stuarty itself.
type jobKiller struct {
	mu  sync.Mutex
	job windows.Handle
}

// attach puts the child in a job object that kills its members when the handle closes.
func attach(cmd *exec.Cmd) (treeKiller, error) {
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

	return &jobKiller{job: job}, nil
}

// Kill terminates every process of the job.
func (j *jobKiller) Kill() {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.job != 0 {
		_ = windows.TerminateJobObject(j.job, 1)
	}
}

// Close releases the handle; the job is empty by then.
func (j *jobKiller) Close() {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.job != 0 {
		_ = windows.CloseHandle(j.job)
		j.job = 0
	}
}
