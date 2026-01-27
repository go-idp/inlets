# Builder
FROM registry.idp.zcorky.com/whatwewant/builder-go:v1.22-1 as builder

WORKDIR /build

COPY go.mod .

COPY go.sum .

ENV GOPROXY=https://goproxy.cn,direct

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -v -o inlets cmd/inlets

FROM registry.idp.zcorky.com/whatwewant/alpine:v3.17-1

ARG VERSION=v1

ENV VERSION=${VERSION}

COPY --from=builder /build/inlets /bin

EXPOSE 8080

CMD inlets server
