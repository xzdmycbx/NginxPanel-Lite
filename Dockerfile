# syntax=docker/dockerfile:1

# --- 1. Build the React SPA ---
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- 2. Build the Go binary (embeds the SPA) ---
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace the placeholder embed dir with the freshly built SPA.
RUN rm -rf internal/web/dist
COPY --from=web /web/dist ./internal/web/dist
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -tags 'osusergo,netgo' -ldflags '-s -w' -o /out/panel ./cmd/server

# --- 3. Minimal runtime ---
# Runs as root: the panel writes the shared nginx config/cert volumes and talks
# to /var/run/docker.sock. See README for the docker-socket-proxy hardening path.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/panel /panel
EXPOSE 8080
ENTRYPOINT ["/panel"]
