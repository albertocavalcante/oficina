# ── Build UI ──────────────────────────────────────────────
FROM docker.io/oven/bun:1-alpine AS ui

WORKDIR /src/ui
COPY ui/package.json ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY ui/ .
RUN bun run build

# ── Build server binary ──────────────────────────────────
FROM docker.io/library/golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd/server

# Non-root user for scratch.
RUN echo "nonroot:x:65534:65534:nonroot:/:" > /tmp/passwd && \
    echo "nonroot:x:65534:" > /tmp/group

# ── Final ────────────────────────────────────────────────
FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /tmp/passwd /etc/passwd
COPY --from=build /tmp/group /etc/group
COPY --from=build /bin/server /server
COPY --from=ui /src/ui/dist /ui/dist

USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/server"]
