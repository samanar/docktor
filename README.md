# docktor

<p align="center">
  <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT">
  <img src="https://img.shields.io/badge/go-%3E%3D1.21-00ADD8.svg" alt="Go >= 1.21">
</p>

A keyboard-driven terminal UI for Docker. Manage containers, images, volumes, and networks — monitor real-time resource usage, browse logs, and run Compose commands, all from the comfort of your terminal.

Built with [Go](https://go.dev), [Bubble Tea](https://github.com/charmbracelet/bubbletea), and the [Docker Engine API SDK](https://pkg.go.dev/github.com/docker/docker/client).

---

## Features

- **Three-pane layout** — Navigator, overview, and detail panel visible at once
- **Compose-aware** — Automatically groups containers by Docker Compose project; collapse/expand groups with a single key
- **Real-time stats** — Live CPU, memory, network I/O, and disk I/O per container, refreshed every second
- **Vim-style navigation** — `j`/`k`, `gg`/`G`, `Ctrl+d`/`Ctrl+u`, `/` search with `n` for next match
- **Single-key actions** — Start, stop, restart, and kill containers without leaving the keyboard
- **Log viewer** — Browse container logs with vim scroll keys; toggle follow mode to stream logs live; search within logs
- **Multi-resource tabs** — Switch between **Containers**, **Images**, **Volumes**, and **Networks** tabs
- **Image layer history** — Inspect individual image layers with creation dates and sizes
- **Volume file usage** — Drill into volumes to see per-file and per-directory disk usage
- **Network inspection** — View full network details including subnet, gateway, and attached containers
- **Compose commands** — Run `docker compose up/down/pull/build/restart` directly from the TUI
- **Bulk actions** — Stop all containers, remove all containers, or prune exited containers in one step
- **Mouse support** — Click to focus panes and select items

---

## Installation

### Prerequisites

- [Go](https://go.dev/dl/) **1.21** or later
- [Docker](https://docs.docker.com/engine/install/) running and accessible from the CLI (`docker ps` should work)

### From source

```bash
git clone https://github.com/samanar/docktor.git
cd docktor
make build
./bin/docktor
```

### Install to `$GOPATH/bin`

```bash
go install github.com/samanar/docktor/cmd/docktor@latest
```

Then run `docktor` from anywhere.

---

## Usage

```bash
docktor
```

Or during development:

```bash
make run
```

### Layout

```
┌───────────────────────────────┬──────────────────────┐
│                               │                      │
│     Resource Navigator        │    Quick Overview    │
│     (Containers / Images      │    (details for      │
│      / Volumes / Networks)    │     selected item)   │
│                               │                      │
├───────────────────────────────┴──────────────────────┤
│                                                      │
│                   Detail Panel                       │
│           (logs / layers / file usage)               │
│                                                      │
└──────────────────────────────────────────────────────┘
```

### Keyboard shortcuts

#### Navigator pane (focus `1`)


| Key                   | Action                                                |
| ----------------------- | ------------------------------------------------------- |
| `j` / `↓`            | Move down                                             |
| `k` / `↑`            | Move up                                               |
| `g` `g`               | Go to first row                                       |
| `G`                   | Go to last row                                        |
| `Ctrl+d`              | Half-page down                                        |
| `Ctrl+u`              | Half-page up                                          |
| `Space`               | Toggle group collapse / expand                        |
| `/`                   | Search containers by name                             |
| `n`                   | Next search match                                     |
| `Enter`               | Select item (view logs, layers, or details)           |
| `s`                   | Start container                                       |
| `x`                   | Stop container                                        |
| `r`                   | Restart container                                     |
| `K`                   | Kill container                                        |
| `u`                   | Compose up (on a Compose group)                       |
| `d`                   | Compose down (on a Compose group)                     |
| `p`                   | Compose pull (on a Compose group)                     |
| `b`                   | Bulk actions dialog                                   |
| `c` / `i` / `v` / `N` | Switch tabs: Containers / Images / Volumes / Networks |
| `q`                   | Quit                                                  |

#### Log / detail viewer (focus `3`)


| Key        | Action                           |
| ------------ | ---------------------------------- |
| `j` / `↓` | Scroll down                      |
| `k` / `↑` | Scroll up                        |
| `g`        | Go to top                        |
| `G`        | Go to bottom                     |
| `Ctrl+d`   | Half-page down                   |
| `Ctrl+u`   | Half-page up                     |
| `f`        | Toggle follow mode (stream logs) |
| `/`        | Search within logs               |
| `n`        | Next log search match            |

#### Global


| Key                    | Action                            |
| ------------------------ | ----------------------------------- |
| `1` / `2` / `3`        | Focus navigator / overview / logs |
| `Tab`                  | Cycle focus                       |
| `q` / `Esc` / `Ctrl+c` | Quit                              |

---

## Development

```bash
make build        # Compile binary to ./bin/docktor
make run          # Build and run
make test         # Run all tests
make test-verbose # Run tests with verbose output
make coverage     # Run tests with coverage report (HTML)
make fmt          # Format all Go source files
make vet          # Run go vet
make tidy         # Tidy module dependencies
make lint         # Run golangci-lint
make clean        # Remove build artifacts
make install      # Install binary to $GOPATH/bin
```

---

## Architecture

```
docktor/
├── cmd/docktor/main.go       # Entry point — initializes theme and Bubble Tea program
├── internal/
│   ├── config/               # Configuration (future: user themes, key bindings)
│   ├── docker/               # Docker client layer
│   │   ├── interface.go      # Client interface (all Docker operations)
│   │   ├── client.go         # Domain types (Container, Image, Volume, etc.)
│   │   ├── sdk_client.go     # Docker Engine API SDK implementation
│   │   └── stats.go          # Stats types and merge helpers
│   └── ui/                   # Terminal UI layer (Bubble Tea)
│       ├── app.go            # Root model, state, and update loop
│       ├── app_view.go       # View rendering (layout composition)
│       ├── app_helpers.go    # Overview renderers (volumes, images, networks)
│       ├── app_dialogs.go    # Modal dialogs (bulk actions)
│       ├── pane.go           # Navigator pane model, messages, and update
│       ├── pane_rows.go      # Row builders for each resource tab
│       ├── table.go          # Generic table component with column definitions
│       ├── colors.go         # Theme struct — all colors in one place
│       └── *_test.go         # Unit tests
├── Makefile                  # Build, test, and dev commands
└── go.mod                    # Go module definition
```

The project follows a clean separation of concerns:

- **`docker/`** — Abstracts all Docker daemon communication behind a `Client` interface, making it testable and swappable.
- **`ui/`** — Pure Bubble Tea model with no Docker logic; it only calls the `Client` interface. All rendering is driven by the `Theme` struct for easy customization.

---

## Tech stack


| Component     | Library                                                                     |
| --------------- | ----------------------------------------------------------------------------- |
| Language      | [Go](https://go.dev)                                                        |
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea)                    |
| Styling       | [Lip Gloss](https://github.com/charmbracelet/lipgloss)                      |
| Docker API    | [Docker Engine API SDK](https://pkg.go.dev/github.com/docker/docker/client) |

---

## License

[MIT](LICENSE)
