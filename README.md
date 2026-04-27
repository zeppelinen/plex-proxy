# plex-proxy

`plex-proxy` exposes a remote Plex server to a trusted local network through an
OpenSSH tunnel. It publishes a local HTTP proxy on TCP `32400` and answers Plex
GDM discovery requests on UDP `32410`, `32412`, `32413`, and `32414`.

The utility is intended for trusted LAN use. Do not expose the proxied Plex
listener directly to the public internet.

## Build

```sh
make build
```

## Configure

Start from `examples/config.yaml`. The SSH target uses the normal OpenSSH client,
so keys, ssh-agent, `~/.ssh/config`, and jump hosts work as they do with `ssh`.

```sh
plex-proxy config validate -config examples/config.yaml
plex-proxy serve -config examples/config.yaml
```

Required settings:

- `ssh.target`: OpenSSH destination, such as `user@jump-host`.
- `plex.remote_host`: Plex host as seen from the SSH target.
- `plex.remote_port`: Plex port, normally `32400`.
- `plex.server_name`: name advertised to local players.

Useful environment overrides:

- `PLEX_PROXY_SSH_TARGET`
- `PLEX_PROXY_REMOTE_HOST`
- `PLEX_PROXY_REMOTE_PORT`
- `PLEX_PROXY_SERVER_NAME`
- `PLEX_PROXY_LISTEN`
- `PLEX_PROXY_ADVERTISE_HOST`

## Tests

```sh
make test
make e2e
```

`make e2e` uses Docker Compose to run an SSH server, a fake Plex server, the
proxy, and a test client on the same Compose network.
