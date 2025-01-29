# duh

A minimalist Docker UI that doesn't get in your way.

## What

- Real-time container stats
- Start/Stop controls
- Memory-based sorting
- Dark mode (because your eyes matter)
- Zero config (because life's too short)

## Install

macOS:
```bash
brew install yarlson/duh/duh
```

Manual:
```bash
# Pick your release at https://github.com/yarlson/duh/releases
curl -L https://github.com/yarlson/duh/releases/latest/download/duh_<version>_<os>_<arch>.tar.gz | tar xz
sudo mv duh /usr/local/bin/
```

Docker:
```bash
docker run -d \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -p 4242:4242 \
  yarlson/duh
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

[MIT](LICENSE)
