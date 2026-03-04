// Package daemon manages the background daemon process for executing deferred tasks.
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Ten-James/defer/internal/storage"
)

const checkInterval = 5 * time.Second

// IsRunning checks if the daemon is currently running.
func IsRunning() bool {
	pid, err := GetPID()
	if err != nil {
		return false
	}

	// Check if process exists by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		cleanup()
		return false
	}

	err = process.Signal(syscall.Signal(0))
	if err != nil {
		cleanup()
		return false
	}

	return true
}

// GetPID returns the daemon PID if the PID file exists.
func GetPID() (int, error) {
	data, err := os.ReadFile(storage.PIDFile)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}

	return pid, nil
}

// Start starts the daemon process in the background.
// Returns true if a new daemon was started, false if already running.
func Start() (bool, error) {
	if IsRunning() {
		return false, nil
	}

	// Get the current executable path
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("failed to get executable path: %w", err)
	}

	// Start the daemon as a detached subprocess
	cmd := exec.Command(exe, "__daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}

	// Redirect stdout/stderr to log file
	logFile := fmt.Sprintf("%s/daemon_%s.log", storage.LogsDir, time.Now().Format("20060102"))
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Errorf("failed to open log file: %w", err)
	}

	cmd.Stdout = f
	cmd.Stderr = f

	if err := cmd.Start(); err != nil {
		f.Close()
		return false, fmt.Errorf("failed to start daemon: %w", err)
	}

	// Write PID file
	if err := os.WriteFile(storage.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		f.Close()
		return false, fmt.Errorf("failed to write PID file: %w", err)
	}

	// Release the process so it doesn't become a zombie
	cmd.Process.Release()
	f.Close()

	// Wait a moment to ensure daemon started
	time.Sleep(100 * time.Millisecond)

	return true, nil
}

// Stop stops the daemon process.
func Stop() bool {
	pid, err := GetPID()
	if err != nil {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		cleanup()
		return false
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		cleanup()
		return false
	}

	cleanup()
	return true
}

// EnsureRunning ensures the daemon is running, starting it if needed.
// Returns true if a new daemon was started.
func EnsureRunning() (bool, error) {
	if IsRunning() {
		return false, nil
	}
	return Start()
}

// Run is the main daemon loop. Called when the process is started with __daemon.
func Run() {
	fmt.Fprintf(os.Stdout, "[%s] Daemon started (PID: %d)\n", time.Now().Format(time.RFC3339), os.Getpid())

	// Write PID file
	os.WriteFile(storage.PIDFile, []byte(strconv.Itoa(os.Getpid())), 0o644)

	store, err := storage.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Failed to initialize storage: %v\n", time.Now().Format(time.RFC3339), err)
		return
	}

	// Setup signal handlers for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	running := true

	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stdout, "[%s] Received signal %v, shutting down gracefully\n", time.Now().Format(time.RFC3339), sig)
		running = false
	}()

	for running {
		readyTasks, err := store.GetReadyTasks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Error getting ready tasks: %v\n", time.Now().Format(time.RFC3339), err)
			time.Sleep(checkInterval)
			continue
		}

		if len(readyTasks) > 0 {
			fmt.Fprintf(os.Stdout, "[%s] Found %d ready task(s)\n", time.Now().Format(time.RFC3339), len(readyTasks))

			for _, task := range readyTasks {
				executeTask(store, &task)
				store.RemoveTask(task.ID)
			}
		}

		// Check if there are any remaining tasks
		hasTasks, err := store.HasTasks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Error checking tasks: %v\n", time.Now().Format(time.RFC3339), err)
		}
		if !hasTasks {
			fmt.Fprintf(os.Stdout, "[%s] No more tasks, shutting down daemon\n", time.Now().Format(time.RFC3339))
			break
		}

		time.Sleep(checkInterval)
	}

	cleanup()
	fmt.Fprintf(os.Stdout, "[%s] Daemon stopped\n", time.Now().Format(time.RFC3339))
}

func executeTask(store *storage.Storage, task *storage.Task) {
	ts := time.Now().Format(time.RFC3339)
	fmt.Fprintf(os.Stdout, "[%s] Executing task %s: %s in %s\n", ts, task.ShortID(), task.CommandString(), task.Cwd)

	logFileName := fmt.Sprintf("task_%s_%s.log", task.ShortID(), time.Now().Format("20060102_150405"))
	logFile := fmt.Sprintf("%s/%s", storage.LogsDir, logFileName)

	f, err := os.Create(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Task %s failed to create log file: %v\n", ts, task.ShortID(), err)
		return
	}
	defer f.Close()

	// Write task metadata
	fmt.Fprintf(f, "Task ID: %s\n", task.ID)
	fmt.Fprintf(f, "Scheduled: %s\n", task.ScheduledAt.Format(time.RFC3339))
	fmt.Fprintf(f, "Executed: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "Command: %s\n", task.CommandString())
	fmt.Fprintf(f, "Working Directory: %s\n", task.Cwd)
	fmt.Fprintf(f, "%s\n\n", strings.Repeat("-", 80))

	// Execute the command
	cmd := exec.Command(task.Command, task.Args...)
	cmd.Dir = task.Cwd
	cmd.Stdout = f
	cmd.Stderr = f

	err = cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(f, "\n\nExit code: %d\n", exitErr.ExitCode())
			fmt.Fprintf(os.Stdout, "[%s] Task %s completed with exit code %d\n", time.Now().Format(time.RFC3339), task.ShortID(), exitErr.ExitCode())
		} else {
			fmt.Fprintf(f, "\n\nERROR: %v\n", err)
			fmt.Fprintf(os.Stderr, "[%s] Task %s failed: %v\n", time.Now().Format(time.RFC3339), task.ShortID(), err)
		}
	} else {
		fmt.Fprintf(f, "\n\nExit code: 0\n")
		fmt.Fprintf(os.Stdout, "[%s] Task %s completed with exit code 0\n", time.Now().Format(time.RFC3339), task.ShortID())
	}

	fmt.Fprintf(os.Stdout, "[%s] Output logged to: %s\n", time.Now().Format(time.RFC3339), logFile)
}

func cleanup() {
	os.Remove(storage.PIDFile)
}
