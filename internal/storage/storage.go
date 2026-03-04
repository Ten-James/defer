// Package storage manages task persistence using JSON files.
package storage

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var (
	// DeferDir is the base directory for defer data.
	DeferDir string
	// TasksFile is the path to the tasks JSON file.
	TasksFile string
	// LogsDir is the directory for execution logs.
	LogsDir string
	// PIDFile is the path to the daemon PID file.
	PIDFile string

	once sync.Once
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	DeferDir = filepath.Join(home, ".defer")
	TasksFile = filepath.Join(DeferDir, "tasks.json")
	LogsDir = filepath.Join(DeferDir, "logs")
	PIDFile = filepath.Join(DeferDir, "daemon.pid")
}

// Task represents a deferred task.
type Task struct {
	ID          string    `json:"id"`
	Command     string    `json:"command"`
	Args        []string  `json:"args"`
	ScheduledAt time.Time `json:"scheduled_at"`
	CreatedAt   time.Time `json:"created_at"`
	Cwd         string    `json:"cwd"`
}

// newUUID generates a random UUID v4 string.
func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewTask creates a new task with a generated ID.
func NewTask(command string, args []string, scheduledAt time.Time, cwd string) *Task {
	return &Task{
		ID:          newUUID(),
		Command:     command,
		Args:        args,
		ScheduledAt: scheduledAt,
		CreatedAt:   time.Now(),
		Cwd:         cwd,
	}
}

// ShortID returns the first 8 characters of the task ID.
func (t *Task) ShortID() string {
	if len(t.ID) >= 8 {
		return t.ID[:8]
	}
	return t.ID
}

// CommandString returns the full command string for display.
func (t *Task) CommandString() string {
	if len(t.Args) == 0 {
		return t.Command
	}
	s := t.Command
	for _, arg := range t.Args {
		s += " " + arg
	}
	return s
}

type tasksFile struct {
	Tasks []Task `json:"tasks"`
}

// Storage manages task persistence using a JSON file.
type Storage struct {
	tasksFile string
	mu        sync.Mutex
}

// New creates a new Storage instance and ensures the storage directory exists.
func New() (*Storage, error) {
	s := &Storage{
		tasksFile: TasksFile,
	}
	if err := s.ensureDirs(); err != nil {
		return nil, fmt.Errorf("failed to create storage directories: %w", err)
	}
	return s, nil
}

func (s *Storage) ensureDirs() error {
	if err := os.MkdirAll(DeferDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(LogsDir, 0o755); err != nil {
		return err
	}
	return nil
}

func (s *Storage) readTasks() ([]Task, error) {
	data, err := os.ReadFile(s.tasksFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tf tasksFile
	if err := json.Unmarshal(data, &tf); err != nil {
		// Corrupted file, start fresh
		fmt.Fprintf(os.Stderr, "Warning: Corrupted tasks file, starting fresh. Error: %v\n", err)
		return nil, nil
	}

	return tf.Tasks, nil
}

func (s *Storage) writeTasks(tasks []Task) error {
	tf := tasksFile{Tasks: tasks}
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then atomic rename
	tmpFile := s.tasksFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tmpFile, s.tasksFile)
}

// AddTask adds a new task to storage.
func (s *Storage) AddTask(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.readTasks()
	if err != nil {
		return err
	}

	tasks = append(tasks, *task)
	return s.writeTasks(tasks)
}

// GetTasks returns all tasks sorted by scheduled time.
func (s *Storage) GetTasks() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.readTasks()
	if err != nil {
		return nil, err
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ScheduledAt.Before(tasks[j].ScheduledAt)
	})

	return tasks, nil
}

// RemoveTask removes a task by ID.
func (s *Storage) RemoveTask(taskID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.readTasks()
	if err != nil {
		return false, err
	}

	newTasks := make([]Task, 0, len(tasks))
	found := false
	for _, t := range tasks {
		if t.ID != taskID {
			newTasks = append(newTasks, t)
		} else {
			found = true
		}
	}

	if found {
		return true, s.writeTasks(newTasks)
	}
	return false, nil
}

// RemoveTaskByIndex removes a task by its index in the sorted list.
func (s *Storage) RemoveTaskByIndex(index int) (*Task, error) {
	tasks, err := s.GetTasks()
	if err != nil {
		return nil, err
	}

	if index < 0 || index >= len(tasks) {
		return nil, fmt.Errorf("invalid index %d, valid range: 0-%d", index, len(tasks)-1)
	}

	task := tasks[index]
	ok, err := s.RemoveTask(task.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("failed to remove task %s", task.ShortID())
	}

	return &task, nil
}

// GetReadyTasks returns tasks whose scheduled time has passed.
func (s *Storage) GetReadyTasks() ([]Task, error) {
	tasks, err := s.GetTasks()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var ready []Task
	for _, t := range tasks {
		if !t.ScheduledAt.After(now) {
			ready = append(ready, t)
		}
	}

	return ready, nil
}

// HasTasks returns true if there are any pending tasks.
func (s *Storage) HasTasks() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.readTasks()
	if err != nil {
		return false, err
	}

	return len(tasks) > 0, nil
}

// ClearAllTasks removes all tasks.
func (s *Storage) ClearAllTasks() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeTasks(nil)
}
