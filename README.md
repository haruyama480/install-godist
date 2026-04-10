# install-godist

## Installation

with go:
```bash
go install github.com/haruyama480/install-godist@latest
```

## Usage

```
install-godist [flags]
```

### Flags

- `-version` Go version to install (`latest`, `1.22`, `1.22.1`, default: `latest`)
- `-dest` Extraction destination directory (default: `./`)
- `-os` Target OS (default: current GOOS)
- `-arch` Target architecture (default: current GOARCH)
- `-unstable` Include non-stable releases (`rc`, `beta`, default: false)

### Examples

```bash
# Install latest stable Go for current platform
install-godist -version latest -dest ./go

# Install latest patch of Go 1.22 for macOS arm64
install-godist -version 1.22 -os darwin -arch arm64 -dest ./go-1.22

# Install exact version
install-godist -version 1.22.5 -dest /usr/local
```

### Notes

- `-version 1.22` resolves to the latest available `go1.22.x`.
- Without `-unstable`, only stable releases are considered. `rc` and `beta` versions are ignored.
- Use write permissions for `-dest` (e.g. `/usr/local` may require `sudo`).
