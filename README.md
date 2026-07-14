# docktor

A keyboard-driven terminal UI for Docker — manage containers, monitor real-time resource usage, and browse logs, all from the comfort of your terminal.

Built with [Go](https://go.dev) and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)

---

## Features

- **Three-pane layout** — Navigator, overview, and logs visible at once
- **Compose-aware** — Automatically groups containers by Docker Compose project
- **Real-time stats** — Live CPU, memory, network I/O, and disk usage per container
- **Vim-style navigation** — `j`/`k`, `gg`/`G`, `Ctrl+d`/`Ctrl+u`, `/` search
- **Single-key actions** — Start, stop, restart, and kill containers with one keystroke
- **Log viewer** — Browse the last 200 lines of container logs with vim scroll keys
- **Search** — Press `/` to search containers by name, `n` for next match

---

## Installation

### From source

```bash
git clone https://github.com/samanar/docktor.git
cd docktor
make build
```

Then run:

```bash
./bin/docktor
```

Or install to your `$GOPATH/bin`:

```bash
go install ./cmd/docktor
```

### Prerequisites

- [Go](https://go.dev/dl/) 1.21+
- [Docker](https://docs.docker.com/engine/install/) running and accessible from the CLI

---

## Usage

Run docktor from your terminal:

```bash
docktor
```

Or with `go run`:

```bash
make run
```

### Keyboard shortcuts

#### Navigator pane (focus `1`)

| Key                           | Action                                           |
| ----------------------------- | ------------------------------------------------ |
| `j` / `↓`                | Move down                                        |
| `k` / `↑`                | Move up                                          |
| `gg`                        | Go to first row                                  |
| `G`                         | Go to last row                                   |
| `Ctrl+d`                    | Half-page down                                   |
| `Ctrl+u`                    | Half-page up                                     |
| `Space`                     | Toggle group collapse                            |
| `/`                         | Search containers by name                        |
| `n`                         | Next search match                                |
| `s`                         | Start container                                  |
| `x`                         | Stop container                                   |
| `r`                         | Restart container                                |
| `K`                         | Kill container                                   |
| `Enter`                     | Select container (view logs + overview)          |
| `c` / `i` / `v` / `N` | Switch tabs (Containers/Images/Volumes/Networks) |
| `q`                         | Quit                                             |

#### Log viewer (focus `3`)

| Key            | Action         |
| -------------- | -------------- |
| `j` / `↓` | Scroll down    |
| `k` / `↑` | Scroll up      |
| `g`          | Go to top      |
| `G`          | Go to bottom   |
| `Ctrl+d`     | Half-page down |
| `Ctrl+u`     | Half-page up   |

#### Global

| Key                          | Action                            |
| ---------------------------- | --------------------------------- |
| `1` / `2` / `3`        | Focus navigator / overview / logs |
| `Tab`                      | Cycle focus                       |
| `q` / `Esc` / `Ctrl+c` | Quit                              |

---


## Roadmap

- [X] Container listing with Compose grouping
- [X] Real-time CPU / memory / network / disk stats
- [X] Container actions (start, stop, restart, kill)
- [X] Log viewer with vim navigation
- [X] Container overview pane with resource details
- [X] Vim-style navigation and search
- [ ] Images, volumes, and networks tabs
- [ ] Docker Compose integration (up/down)
- [ ] Filter by state (running/stopped)
- [ ] Custom themes
- [ ] Mouse support
