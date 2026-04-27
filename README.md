# plex-proxy

`plex-proxy` makes a remote Plex Media Server appear on a trusted local network.
It opens a supervised OpenSSH tunnel to the remote network, exposes Plex locally
on TCP `32400`, and answers Plex GDM discovery requests so local players can find
the proxied server automatically.

The utility is intended for trusted LAN use. Do not expose the proxied Plex
listener directly to the public internet.

## Features

- Supervised OpenSSH tunnel using your normal SSH config, keys, agent, and jump hosts.
- Local Plex HTTP reverse proxy on TCP `32400`.
- Plex GDM discovery replies on UDP `32410`, `32412`, `32413`, and `32414`.
- Optional raw TCP forwarding for additional Plex ports such as DLNA `32469`.
- Health and readiness endpoints.
- Linux and macOS release binaries.

## Install

Install the latest release artifact for your platform into `/usr/local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/zeppelinen/plex-proxy/main/install.sh | bash
```

To install somewhere else:

```sh
curl -fsSL https://raw.githubusercontent.com/zeppelinen/plex-proxy/main/install.sh | BIN_DIR="$HOME/.local/bin" bash
```

On macOS, the installer prints follow-up steps for System Settings > Privacy &
Security if Gatekeeper blocks the downloaded binary.

PR builds also upload temporary artifacts that expire after 7 days. To download
one with GitHub CLI:

```sh
gh run list --repo zeppelinen/plex-proxy --event pull_request
gh run download RUN_ID --repo zeppelinen/plex-proxy --name plex-proxy-pr-PR_NUMBER
```

## Build From Source

```sh
git clone https://github.com/zeppelinen/plex-proxy.git
cd plex-proxy
make build
```

The release build script produces:

- `plex-proxy_<version>_linux_amd64.tar.gz`
- `plex-proxy_<version>_linux_arm64.tar.gz`
- `plex-proxy_<version>_darwin_arm64.tar.gz`
- `checksums.txt`

## Commands

```sh
plex-proxy serve -config /etc/plex-proxy/config.yaml
plex-proxy config validate -config /etc/plex-proxy/config.yaml
plex-proxy version
```

If no command is provided, `plex-proxy` runs `serve`.

## Configuration

Start from [examples/config.yaml](examples/config.yaml).

```yaml
ssh:
  # SSH host from ~/.ssh/config, or user@host.
  target: myhome-srv
  local_listen: 127.0.0.1:0
  # identity_file: /home/plex-proxy/.ssh/id_ed25519
  # config_file: /home/plex-proxy/.ssh/config
  # extra_args: ["-J", "bastion.example.com"]

plex:
  # Plex IP or hostname as seen from the SSH target.
  remote_host: 192.168.1.110
  remote_port: 32400
  server_name: Remote Plex
  # Set these to the real values from http://127.0.0.1:32400/identity.
  machine_id: real-plex-machine-identifier
  version: real-plex-version

proxy:
  listen: 0.0.0.0:32400

gdm:
  enabled: true
  # Set this to the plex-proxy machine's LAN IP when auto-detection picks the wrong address.
  # advertise_host: 192.168.1.10
  ports: [32410, 32412, 32413, 32414]

health:
  listen: 127.0.0.1:8080

forward:
  - name: dlna
    enabled: false
    listen: 0.0.0.0:32469
    target_host: 127.0.0.1
    target_port: 32469

log_format: text
```

Required fields:

- `ssh.target`: OpenSSH destination, for example `user@jump-host.example.com`.
- `plex.remote_host`: Plex host as seen from the SSH target.
- `plex.remote_port`: Plex port, normally `32400`.
- `plex.server_name`: name advertised to local players.

For best Plex client discovery behavior, set `plex.machine_id` and
`plex.version` to the values returned by the real server's proxied
`/identity` endpoint. The GDM `Resource-Identifier` should match the Plex
`machineIdentifier`; placeholder IDs can make clients discover a server they
then cannot match to the HTTP identity endpoint.

Useful environment overrides:

- `PLEX_PROXY_SSH_TARGET`
- `PLEX_PROXY_SSH_CONFIG_FILE`
- `PLEX_PROXY_SSH_IDENTITY_FILE`
- `PLEX_PROXY_REMOTE_HOST`
- `PLEX_PROXY_REMOTE_PORT`
- `PLEX_PROXY_SERVER_NAME`
- `PLEX_PROXY_MACHINE_ID`
- `PLEX_PROXY_LISTEN`
- `PLEX_PROXY_ADVERTISE_HOST`
- `PLEX_PROXY_HEALTH_LISTEN`

## Examples

Validate a config:

```sh
plex-proxy config validate -config ./examples/config.yaml
```

Run in the foreground:

```sh
PLEX_PROXY_SSH_TARGET=user@jump-host.example.com \
PLEX_PROXY_REMOTE_HOST=127.0.0.1 \
PLEX_PROXY_SERVER_NAME="Remote Plex" \
plex-proxy serve -config ./examples/config.yaml
```

Install as a systemd service:

```sh
sudo useradd --system --home /var/lib/plex-proxy --shell /usr/sbin/nologin plex-proxy
sudo install -d -o plex-proxy -g plex-proxy /etc/plex-proxy /var/lib/plex-proxy/.ssh
sudo install -m 0644 examples/config.yaml /etc/plex-proxy/config.yaml
sudo install -m 0644 examples/plex-proxy.service /etc/systemd/system/plex-proxy.service
sudo systemctl daemon-reload
sudo systemctl enable --now plex-proxy
```

## Tests

```sh
make test
make e2e
```

`make e2e` uses Docker Compose to run an SSH server, a fake Plex server, the
proxy, and a test client on the same Compose network.
