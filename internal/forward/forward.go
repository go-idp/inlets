package forward

import (
	"net"

	"github.com/go-zoox/logger"
)

type Forward struct {
	source string
	target string
}

func New(source string, target string) *Forward {
	return &Forward{source, target}
}

func (t *Forward) Start() {
	logger.Info("Start tcp forward %s => %s", t.source, t.target)

	ch := make(chan *net.TCPConn, 10)

	go waitConnection(ch, t.target)

	NewServer(ch, t.source).Start()
}

func waitConnection(sourceConnCh chan *net.TCPConn, target string) {
	count := 0

	for sourceConn := range sourceConnCh {
		count += 1

		logger.Info("[%d] connection coming ...", count)
		targetClient := NewClient(target)

		targetClient.Connect(sourceConn)
	}
}
