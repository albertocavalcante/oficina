# Compose Example

Run oficina locally with [Docker Compose](https://docs.docker.com/compose/) or [Podman Compose](https://github.com/containers/podman-compose). Starts a server and two agents that build from source.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) with Docker Compose v2, **or** [Podman](https://podman.io/) with `podman-compose`

## Usage

From the repository root:

```bash
just compose-up
```

Or directly:

```bash
cd examples/compose
docker compose up --build
```

The dashboard is at [http://localhost:8080](http://localhost:8080). Two agents (`agent-01`, `agent-02`) register automatically.

## Stop

```bash
just compose-down
```

Or:

```bash
docker compose down
```

## Using Pre-built Images

To skip the build step, edit `compose.yaml`: comment out each `build:` block and uncomment the `image:` line. Make sure the images exist locally (`just container`) or are available from a registry.
