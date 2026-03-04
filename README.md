# defer

[![CI](https://github.com/Ten-James/defer/actions/workflows/ci.yml/badge.svg)](https://github.com/Ten-James/defer/actions/workflows/ci.yml)
[![Nightly](https://github.com/Ten-James/defer/actions/workflows/nightly.yml/badge.svg)](https://github.com/Ten-James/defer/releases/tag/nightly)
[![Go Report Card](https://goreportcard.com/badge/github.com/Ten-James/defer)](https://goreportcard.com/report/github.com/Ten-James/defer)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Schedule any command to run later.** No crontab editing, no config files, no dependencies.

```bash
defer 5m echo "Hello World"
defer 2h python backup.py
defer 1d ./cleanup.sh
```

`defer` runs a lightweight background daemon that executes your commands when the time comes, then shuts itself down. Tasks persist across terminal sessions and reboots.

---

## Install

### From source

```bash
go install github.com/Ten-James/defer@latest
```

### From release binaries

Download the latest binary from [Releases](https://github.com/Ten-James/defer/releases):

```bash
# macOS (Apple Silicon)
curl -sL https://github.com/Ten-James/defer/releases/download/nightly/defer-darwin-arm64.tar.gz | tar xz
mv defer-darwin-arm64 /usr/local/bin/defer

# macOS (Intel)
curl -sL https://github.com/Ten-James/defer/releases/download/nightly/defer-darwin-amd64.tar.gz | tar xz
mv defer-darwin-amd64 /usr/local/bin/defer

# Linux (x86_64)
curl -sL https://github.com/Ten-James/defer/releases/download/nightly/defer-linux-amd64.tar.gz | tar xz
mv defer-linux-amd64 /usr/local/bin/defer

# Linux (ARM64)
curl -sL https://github.com/Ten-James/defer/releases/download/nightly/defer-linux-arm64.tar.gz | tar xz
mv defer-linux-arm64 /usr/local/bin/defer
```

### Build from source

```bash
git clone https://github.com/Ten-James/defer.git
cd defer
make build
# binary is at ./defer
```

## Usage

### Schedule a command

```bash
defer <time> <command> [args...]
```

The time argument supports human-readable formats:

| Format | Duration |
|--------|----------|
| `30s`, `30sec` | 30 seconds |
| `5m`, `5min` | 5 minutes |
| `2h`, `2hr` | 2 hours |
| `1d`, `1day` | 1 day |
| `1h30m` | Combined units |
| `1.5h` | Decimal values |

### Manage tasks

```bash
defer list              # List all scheduled tasks
defer remove <index>    # Remove a task by its index
defer status            # Show daemon and task status
defer version           # Print version
```

### Example: list output

```
Deferred tasks (3):

#    Scheduled            Relative        Command
------------------------------------------------------------
0    2026-03-04 17:30:00  in 25m          echo Hello World
1    2026-03-04 19:00:00  in 1h 55m       python backup.py
2    2026-03-05 16:05:00  in 23h 30m      ./cleanup.sh

Daemon: Running (PID: 12345)
```

## How it works

```
  you                     defer                    daemon
   |                        |                        |
   |-- defer 5m echo hi --> |                        |
   |                        |-- save task to disk --> |
   |                        |-- start daemon ------> |  (if not running)
   |   "Task scheduled"  <--|                        |
   |                                                 |-- [poll every 5s]
   |                                                 |-- time's up!
   |                                                 |-- exec: echo hi
   |                                                 |-- log output
   |                                                 |-- no tasks left?
   |                                                 |-- shut down
```

1. **Schedule** -- `defer` parses the time, saves the task to `~/.defer/tasks.json`, and ensures the daemon is running.
2. **Execute** -- The daemon polls every 5 seconds, runs commands when their time arrives, and logs output to `~/.defer/logs/`.
3. **Shutdown** -- When no tasks remain, the daemon exits automatically.

Commands execute in the working directory where they were scheduled, so relative paths and local config files work as expected.

## File layout

```
~/.defer/
  tasks.json       # Persisted task queue
  daemon.pid       # Daemon process ID
  logs/
    daemon_YYYYMMDD.log              # Daemon activity
    task_<id>_YYYYMMDD_HHMMSS.log   # Per-task stdout/stderr + exit code
```

## Real-world examples

```bash
# Remind yourself to take a break
defer 1h osascript -e 'display notification "Break time!" with title "Reminder"'

# Delayed git push (finish reviewing before it goes out)
defer 2m git push

# Schedule a backup
defer 6h rsync -av ~/documents /backup/

# Delayed API notification
defer 15m curl -X POST -H "Content-Type: application/json" \
  -d '{"status":"deployed"}' https://api.example.com/notify

# Clean up old logs tomorrow
defer 1d find /var/log/app -mtime +30 -delete
```

## Troubleshooting

**Daemon won't start** -- Check permissions on `~/.defer/` and available disk space. Inspect `~/.defer/logs/daemon_*.log`.

**Stale PID file** -- If the daemon crashed, remove the stale PID file and retry:

```bash
rm ~/.defer/daemon.pid
defer 1m echo "test"
```

**View logs** -- Tail the daemon log or inspect individual task logs:

```bash
tail -f ~/.defer/logs/daemon_$(date +%Y%m%d).log
ls ~/.defer/logs/task_*
```

## Requirements

- **OS:** Linux or macOS (uses process sessions for daemon management)
- **Build:** Go 1.21+
- **Runtime:** Zero dependencies -- single static binary

## Contributing

Contributions are welcome. Please open an issue to discuss larger changes before submitting a PR.

```bash
# Development workflow
make fmt       # Format code
make vet       # Run static analysis
make test      # Run tests
make build     # Build binary
```

## Roadmap

- [ ] Recurring tasks (cron-like scheduling)
- [ ] Task dependencies (run B after A completes)
- [ ] Notifications on completion (desktop / webhook)
- [ ] Task timeout limits
- [ ] Retry on failure

## License

[MIT](LICENSE)
