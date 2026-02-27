# jenkins-cli

A k9s-inspired terminal UI for Jenkins. Navigate environments, services, build history and trigger deployments without leaving your terminal.

## Install

### macOS / Linux — download binary (no Go required)

Go to the [Releases](https://github.com/marti-batuhankutluay/jenkins-cli/releases/latest) page, download the archive for your platform, and move the binary to your PATH:

```bash
# macOS Apple Silicon
curl -L https://github.com/marti-batuhankutluay/jenkins-cli/releases/latest/download/jenkins-cli_darwin_arm64.tar.gz | tar xz
sudo mv jenkins-cli /usr/local/bin/

# macOS Intel
curl -L https://github.com/marti-batuhankutluay/jenkins-cli/releases/latest/download/jenkins-cli_darwin_amd64.tar.gz | tar xz
sudo mv jenkins-cli /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/marti-batuhankutluay/jenkins-cli/releases/latest/download/jenkins-cli_linux_amd64.tar.gz | tar xz
sudo mv jenkins-cli /usr/local/bin/
```

### go install

If you have Go 1.22+ installed:

```bash
go install github.com/marti-batuhankutluay/jenkins-cli@latest
```

Then make sure your Go bin directory is in `PATH` (add to `~/.zshrc` or `~/.bashrc`):

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

## First Run

On first launch you'll be prompted for your Jenkins credentials:

| Field | Description |
|-------|-------------|
| **Jenkins URL** | Your Jenkins instance URL, e.g. `https://jenkins.example.com` |
| **Username** | Your Jenkins username |
| **API Token** | Jenkins → profile (top right) → *Configure* → *API Token* → *Add new Token* |

Credentials are saved to `~/.config/jenkins-cli/config.yaml` and reused on subsequent runs.
To reset (e.g. after a token change):

```bash
rm ~/.config/jenkins-cli/config.yaml
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` &nbsp; or &nbsp; `k` / `j` | Navigate |
| `Enter` | Select / drill down |
| `Esc` / `Backspace` | Go back |
| `/` | Filter list |
| `b` | Trigger build |
| `d` | Trigger deploy |
| `l` | Open build log |
| `w` | Active builds |
| `r` | Refresh |
| `?` | Toggle help |
| `q` / `Ctrl+C` | Quit |
