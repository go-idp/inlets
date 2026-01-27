package main

import (
	"os"

	"github.com/go-idp/inlets"
	"github.com/go-zoox/logger"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:    "inlets",
		Usage:   "高可用 inlets 客户端的 Go 实现，负责与云端隧道服务通过 WebSocket 建立长连接，并把本地 HTTP/TCP 服务安全地暴露到公网。",
		Version: inlets.Version,
		Commands: []*cli.Command{
			Client(),
			Server(),
			Forward(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatal("%s", err.Error())
	}
}
