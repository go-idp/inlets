# Builder
FROM --platform=$BUILDPLATFORM whatwewant/builder-go:v1.22-1 as builder

WORKDIR /build

COPY go.mod ./

COPY go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 \
  GOOS=$TARGETOS \
  GOARCH=$TARGETARCH \
  go build \
  -trimpath \
  -ldflags '-w -s -buildid=' \
  -v -o inlets ./cmd/inlets

# Server
FROM whatwewant/alpine:v3-1

LABEL MAINTAINER="Zero<tobewhatwewant@gmail.com>"

LABEL org.opencontainers.image.source="https://github.com/go-idp/inlets"

COPY --from=builder /build/inlets /bin

CMD inlets server
