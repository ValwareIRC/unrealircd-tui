# UnrealIRCd Terminal Manager

[![Go Version](https://img.shields.io/badge/Go-1.25.4-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

A powerful terminal-based user interface (TUI) for managing UnrealIRCd IRC servers. Built with Go and the tview library, this tool provides comprehensive installation, configuration, module management, and remote control capabilities for UnrealIRCd servers.

## Features

### 🛠️ Installation & Setup
- **Automatic Source Detection**: Scans your system for existing UnrealIRCd source directories
- **One-Click Installation**: Download and install UnrealIRCd with guided configuration
- **Version Management**: Check for updates and switch between installations
- **Configuration Wizard**: Interactive setup with sensible defaults

### 📦 Module Management
- **Module Browser**: Browse and install modules from GitHub
- **Third-Party Modules**: Support for external module repositories
- **Dependency Checking**: Automatically verify module requirements
- **Custom Module Upload**: Install your own modules directly

### 🤖 Script Management
- **Obby Script Support**: Manage IRC scripts with ease
- **Script Editor**: Built-in editor for modifying scripts
- **Installation/Uninstallation**: Simple script lifecycle management

### 🌐 Remote Control (RPC)
- **Real-time Monitoring**: Connect to running servers via WebSocket RPC
- **User Management**: View and manage online users
- **Channel Oversight**: Monitor channels, topics, and member lists
- **Server Statistics**: View server information and uptime
- **Ban Management**: Handle G-lines, K-lines, and Z-lines
- **Log Streaming**: Real-time server log monitoring with filtering

### 🎨 User Interface
- **Terminal-Based**: Full TUI with mouse support
- **Keyboard Navigation**: Efficient keyboard shortcuts
- **Color-Coded**: Intuitive color scheme for different data types
- **Responsive Design**: Adapts to terminal size

## Prerequisites

- **Go 1.25.4 or later** — required to build the tool ([download](https://go.dev/dl/))
- **Git** — to clone the repository
- **Terminal with Unicode support** — e.g. xterm-256color, gnome-terminal, Konsole, iTerm2
- **Internet connection** — for downloading UnrealIRCd source and RPC connectivity
- **C build toolchain** (for compiling UnrealIRCd and modules):
  - `gcc` or `cc`
  - `make`
  - `autoconf`
- **Development libraries** (for compiling UnrealIRCd):
  - OpenSSL/LibreSSL development headers (`libssl-dev` / `openssl-devel`)
  - On Debian/Ubuntu: `sudo apt install build-essential pkg-config libssl-dev libcurl4-openssl-dev`
  - On RHEL/Fedora: `sudo dnf install gcc make openssl-devel libcurl-devel`

## Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/ValwareIRC/unrealircd-tui.git
   cd unrealircd-tui
   ```

2. **Download dependencies and build:**
   ```bash
   go mod download
   go build -o utui
   ```

3. **Run the tool:**
   ```bash
   ./utui
   ```

> **Note:** On first run, if no existing UnrealIRCd installation is detected, the tool will prompt you to download and set one up.

## Usage

### First-Time Setup

When you first run the tool, it will:
1. Scan for existing UnrealIRCd installations
2. If none found, guide you through downloading and installing UnrealIRCd
3. Set up your build directory (default: `~/unrealircd`)

### Main Menu Options

- **Install UnrealIRCd**: Download and configure a new server
- **Check for Updates**: Update your source code to the latest version
- **Module Manager**: Browse and install modules
- **Script Manager**: Manage IRC scripts
- **Remote Control**: Connect to a running server for monitoring
- **Switch Installation**: Change between multiple installations

### Remote Control Setup

To use remote control features:

1. Ensure your UnrealIRCd server has RPC enabled in `unrealircd.conf`. See the docs on how to do this: https://www.unrealircd.org/docs/JSON-RPC
2. Rehash
3. In the tool, select "Remote Control" and configure:
   - WebSocket URL (default: `wss://127.0.0.1:8600/`)
   - RPC username and password

## Configuration

The tool stores configuration in:
- `~/.unrealircd_manager_config` - Tool settings
- `~/.unrealircd_rpc_config` - RPC connection details

### RPC Configuration

The RPC config file contains:
```json
{
  "username": "rpc_user",
  "password": "secure_password",
  "ws_url": "wss://127.0.0.1:8600/"
}
```

## Architecture

```
unrealircd-tui/
├── main.go              # Main application and TUI logic
├── rpc/                 # RPC client and types
│   ├── client.go        # WebSocket RPC communication
│   ├── config.go        # RPC configuration management
│   ├── types.go         # Data structures for RPC responses
│   └── rpc_test.go      # Unit tests
└── ui/                  # User interface components
    └── remote_control.go # Remote control interface
```

## Dependencies

- [tview](https://github.com/rivo/tview) - Terminal UI library
- [unrealircd-rpc-golang](https://github.com/ObsidianIRC/unrealircd-rpc-golang) - UnrealIRCd RPC client
- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket client

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature-name`
3. Make your changes and add tests
4. Run tests: `go test ./...`
5. Commit your changes: `git commit -am 'Add feature'`
6. Push to the branch: `git push origin feature-name`
7. Submit a pull request

## Troubleshooting

### Common Issues

**Program exits immediately / nothing happens**
- This was a known bug in older versions — the tool would silently exit if no UnrealIRCd installation was found. Update to the latest version.
- Ensure you built with `go build -o utui` (not just copied the binary from another machine — it must be compiled for your OS/architecture).
- Run with `file ./utui` to verify it matches your system architecture.

**RPC Connection Failed**
- Verify UnrealIRCd RPC is enabled and listening on the correct port
- Check firewall settings
- Ensure TLS certificates are valid (or disable TLS verification for development)

**Module Installation Failed**
- Check that you have build tools installed (make, gcc)
- Verify source directory permissions
- Ensure all dependencies are met

**Terminal Display Issues**
- Use a Unicode-supporting terminal
- Try increasing terminal font size
- Check for tview compatibility

### Debug Logging

The tool writes debug information to `/tmp/debug.log`. Check this file for detailed error information.

## License

This project is licensed under the GNU General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

## Author

Created by [ValwareIRC](https://github.com/ValwareIRC)

## Acknowledgments

- [UnrealIRCd](https://www.unrealircd.org/) - The IRC daemon this tool manages
- [tview](https://github.com/rivo/tview) - Excellent terminal UI library
- [ObsidianIRC](https://github.com/ObsidianIRC) - RPC library for UnrealIRCd
