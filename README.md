# duh

A minimalist Docker UI that doesn't get in your way.

## What

- Real-time container stats
- Start/Stop controls
- Memory-based sorting
- Dark mode (because your eyes matter)
- Zero config (because life's too short)

## Install

```bash
go install github.com/yarlson/duh@latest
```

## Run

```bash
duh
```

That's it. Really. Browser will open automatically at http://localhost:4242

## Requirements

- Docker daemon
- Docker socket at `/var/run/docker.sock`
- A browser from this decade

## Dev Setup

```bash
git clone https://github.com/yarlson/duh
cd duh
go run .
```

## License

MIT (do whatever you want)
