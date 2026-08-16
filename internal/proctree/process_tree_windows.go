//go:build windows

package proctree

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processTree struct {
	job     windows.Handle
	groupID uint32
}

func configureProcessTree(cmd *exec.Cmd) {
	// Suspension closes the cmd.Start -> AssignProcessToJobObject race: the child cannot
	// spawn a helper until it belongs to the Job Object that descendants inherit.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

func attachProcessTree(cmd *exec.Cmd) (*processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = windows.CloseHandle(job)
		}
	}()

	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		return nil, fmt.Errorf("set job object limits: %w", err)
	}

	pid := uint32(cmd.Process.Pid)
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		pid,
	)
	if err != nil {
		return nil, fmt.Errorf("open child process: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(process)
		return nil, fmt.Errorf("assign child to job object: %w", err)
	}
	_ = windows.CloseHandle(process)

	if err := resumePrimaryThread(pid); err != nil {
		_ = windows.TerminateJobObject(job, 1)
		return nil, err
	}

	cleanup = false
	return &processTree{job: job, groupID: pid}, nil
}

func resumePrimaryThread(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot child threads: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return fmt.Errorf("open child thread: %w", openErr)
		}
		_, resumeErr := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if resumeErr != nil {
			return fmt.Errorf("resume child thread: %w", resumeErr)
		}
		return nil
	}
	if err != nil && !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate child threads: %w", err)
	}
	return errors.New("child process has no primary thread")
}

func (p *processTree) terminate() {
	// CTRL_BREAK lets ffmpeg flush when the process shares a console. Services commonly do
	// not, so failure is expected; TerminateJobObject remains the bounded backstop.
	_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, p.groupID)
}

func (p *processTree) kill()  { _ = windows.TerminateJobObject(p.job, 1) }
func (p *processTree) close() { _ = windows.CloseHandle(p.job) }
