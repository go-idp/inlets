# Admin UI → internal/server/admin/static/dist (embedded with -tags adminui)
FROM node:20-alpine AS admin-ui

WORKDIR /build

COPY admin/package.json admin/pnpm-lock.yaml ./admin/

WORKDIR /build/admin

RUN corepack enable && corepack prepare pnpm@9 --activate \
  && pnpm install --frozen-lockfile

COPY admin/ .

RUN pnpm build

# Go binary with embedded admin SPA
FROM --platform=$BUILDPLATFORM whatwewant/builder-go:v1.25-1 AS builder

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

COPY --from=admin-ui /build/internal/server/admin/static/dist ./internal/server/admin/static/dist

RUN CGO_ENABLED=0 \
  GOOS=$TARGETOS \
  GOARCH=$TARGETARCH \
  go build \
  -tags adminui \
  -trimpath \
  -ldflags '-w -s -buildid=' \
  -v -o inlets ./cmd/inlets

FROM whatwewant/alpine:v3-1

LABEL MAINTAINER="Zero<tobewhatwewant@gmail.com>"

LABEL org.opencontainers.image.source="https://github.com/go-idp/inlets"

COPY --from=builder /build/inlets /bin

CMD inlets server
