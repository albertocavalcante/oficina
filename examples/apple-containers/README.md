# Apple Containers Example

Run oficina locally using [Apple Containers](https://developer.apple.com/documentation/apple-containers) on macOS 26+ (Apple Silicon).

Apple Containers is the native macOS container runtime. It doesn't support Compose files, so `run.sh` handles building, networking, and lifecycle management.

## Prerequisites

- macOS 26 (Tahoe) or later
- Apple Silicon (M1+)
- Apple Containers CLI (`container`) — ships with macOS 26

## Usage

```bash
cd examples/apple-containers
./run.sh
```

The dashboard is at [http://localhost:8080](http://localhost:8080). One agent (`agent-01`) registers automatically.

Press **Ctrl+C** to stop. The script cleans up containers and the network on exit.

## How It Works

The script:

1. Creates a `container network` named `oficina`
2. Builds server and agent images from the repo Containerfiles
3. Starts the server container with port 8080 published
4. Starts an agent container that connects to the server via `server.test` DNS (Apple Containers resolves `<name>.test` for containers on the same network)
5. On exit (Ctrl+C), stops containers and removes the network
