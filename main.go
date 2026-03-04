package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Ten-James/defer/internal/daemon"
	"github.com/Ten-James/defer/internal/storage"
	"github.com/Ten-James/defer/internal/timeparse"
)

const version = "1.0.0"

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	// Check for internal daemon command
	if args[0] == "__daemon" {
		daemon.Run()
		return
	}

	// Handle subcommands
	switch args[0] {
	case "list":
		os.Exit(cmdList())
	case "remove":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: remove requires a task index")
			fmt.Fprintln(os.Stderr, "Usage: defer remove <index>")
			os.Exit(1)
		}
		index, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid index '%s'\n", args[1])
			os.Exit(1)
		}
		os.Exit(cmdRemove(index))
	case "status":
		os.Exit(cmdStatus())
	case "help", "--help", "-h":
		printUsage()
		os.Exit(0)
	case "version", "--version", "-v":
		fmt.Printf("defer %s\n", version)
		os.Exit(0)
	default:
		// Check if first arg looks like a time spec
		if timeparse.LooksLikeTimeSpec(args[0]) {
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "Error: no command specified")
				fmt.Fprintln(os.Stderr, "Usage: defer <time> <command> [args...]")
				os.Exit(1)
			}
			os.Exit(cmdDefer(args[0], args[1], args[2:]))
		} else {
			fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", args[0])
			fmt.Fprintln(os.Stderr, "")
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Println("defer - Schedule command execution after a time delay")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  defer <time> <command> [args...]   Schedule a command")
	fmt.Println("  defer list                         List all deferred tasks")
	fmt.Println("  defer remove <index>               Remove a task by index")
	fmt.Println("  defer status                       Show daemon and task status")
	fmt.Println()
	fmt.Println("Time formats:")
	fmt.Println("  30s, 5m, 2h, 1d                    Single unit")
	fmt.Println("  1h30m, 2d12h                       Combined units")
	fmt.Println("  1.5h                               Decimal values")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  defer 5m echo \"Hello World\"         Run in 5 minutes")
	fmt.Println("  defer 2h python backup.py           Run in 2 hours")
	fmt.Println("  defer 1d ./cleanup.sh               Run in 1 day")
	fmt.Println("  defer 30m curl -X POST https://api.example.com/webhook")
}

func cmdDefer(timeStr, command string, cmdArgs []string) int {
	// Parse the time delay
	delay, err := timeparse.Parse(timeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Calculate scheduled time
	scheduledAt := time.Now().Add(delay)

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to get working directory: %v\n", err)
		return 1
	}

	// Create and store task
	task := storage.NewTask(command, cmdArgs, scheduledAt, cwd)

	store, err := storage.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if err := store.AddTask(task); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save task: %v\n", err)
		return 1
	}

	// Ensure daemon is running
	started, err := daemon.EnsureRunning()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start daemon: %v\n", err)
	}

	// Display confirmation
	fmt.Println("Task scheduled:")
	fmt.Printf("  ID: %s\n", task.ShortID())
	fmt.Printf("  Command: %s\n", task.CommandString())
	fmt.Printf("  Working directory: %s\n", cwd)
	fmt.Printf("  Scheduled for: %s (%s)\n", scheduledAt.Format("2006-01-02 15:04:05"), formatRelativeTime(scheduledAt))

	if started {
		pid, _ := daemon.GetPID()
		fmt.Printf("  Daemon started (PID: %d)\n", pid)
	} else {
		pid, _ := daemon.GetPID()
		fmt.Printf("  Daemon already running (PID: %d)\n", pid)
	}

	return 0
}

func cmdList() int {
	store, err := storage.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	tasks, err := store.GetTasks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if len(tasks) == 0 {
		fmt.Println("No deferred tasks.")
		return 0
	}

	now := time.Now()

	fmt.Printf("Deferred tasks (%d):\n\n", len(tasks))

	// Calculate max command length
	maxCmdLen := 0
	for _, t := range tasks {
		l := len(t.CommandString())
		if l > maxCmdLen {
			maxCmdLen = l
		}
	}
	if maxCmdLen > 50 {
		maxCmdLen = 50
	}

	// Header
	fmt.Printf("%-4s %-20s %-15s %-*s\n", "#", "Scheduled", "Relative", maxCmdLen, "Command")
	fmt.Println(strings.Repeat("-", 4+20+15+maxCmdLen+6))

	// Tasks
	for i, task := range tasks {
		cmdStr := task.CommandString()
		if len(cmdStr) > maxCmdLen {
			cmdStr = cmdStr[:maxCmdLen-3] + "..."
		}

		scheduledStr := task.ScheduledAt.Format("2006-01-02 15:04:05")
		relativeStr := formatRelativeTime(task.ScheduledAt)

		if !task.ScheduledAt.After(now) {
			relativeStr = fmt.Sprintf("READY (%s)", relativeStr)
		}

		fmt.Printf("%-4d %-20s %-15s %s\n", i, scheduledStr, relativeStr, cmdStr)
	}

	fmt.Println()

	// Daemon status
	if daemon.IsRunning() {
		pid, _ := daemon.GetPID()
		fmt.Printf("Daemon: Running (PID: %d)\n", pid)
	} else {
		fmt.Println("Daemon: Not running")
	}

	return 0
}

func cmdRemove(index int) int {
	store, err := storage.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	tasks, err := store.GetTasks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if len(tasks) == 0 {
		fmt.Fprintln(os.Stderr, "No deferred tasks to remove.")
		return 1
	}

	if index < 0 || index >= len(tasks) {
		fmt.Fprintf(os.Stderr, "Error: Invalid index %d. Valid range: 0-%d\n", index, len(tasks)-1)
		return 1
	}

	task := tasks[index]
	removed, err := store.RemoveTask(task.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if removed {
		fmt.Printf("Removed task %d:\n", index)
		fmt.Printf("  Command: %s\n", task.CommandString())
		fmt.Printf("  Was scheduled for: %s\n", task.ScheduledAt.Format("2006-01-02 15:04:05"))

		hasTasks, _ := store.HasTasks()
		if !hasTasks {
			fmt.Println("\nNo more tasks remaining. Daemon will shut down automatically.")
		}
		return 0
	}

	fmt.Fprintf(os.Stderr, "Error: Failed to remove task %d\n", index)
	return 1
}

func cmdStatus() int {
	store, err := storage.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	tasks, err := store.GetTasks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Println("Defer Status:")
	fmt.Println()

	// Daemon status
	if daemon.IsRunning() {
		pid, _ := daemon.GetPID()
		fmt.Printf("Daemon: Running (PID: %d)\n", pid)
	} else {
		fmt.Println("Daemon: Not running")
	}

	fmt.Printf("Total tasks: %d\n", len(tasks))

	if len(tasks) > 0 {
		now := time.Now()
		readyCount := 0
		for _, t := range tasks {
			if !t.ScheduledAt.After(now) {
				readyCount++
			}
		}
		fmt.Printf("Ready to execute: %d\n", readyCount)
		fmt.Printf("Pending: %d\n", len(tasks)-readyCount)

		nextTask := tasks[0]
		fmt.Println()
		fmt.Println("Next task:")
		fmt.Printf("  Command: %s\n", nextTask.CommandString())
		fmt.Printf("  Scheduled: %s (%s)\n", nextTask.ScheduledAt.Format("2006-01-02 15:04:05"), formatRelativeTime(nextTask.ScheduledAt))
	}

	return 0
}

func formatRelativeTime(t time.Time) string {
	now := time.Now()
	d := t.Sub(now)

	if d > 0 {
		return "in " + timeparse.FormatDuration(d)
	}
	return timeparse.FormatDuration(-d) + " ago"
}
