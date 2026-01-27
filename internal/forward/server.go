package forward

import (
	"net"

	"github.com/go-zoox/logger"
)

type Server struct {
	addr string
	conn chan *net.TCPConn
}

func NewServer(conn chan *net.TCPConn, addr string) *Server {
	return &Server{addr: addr, conn: conn}
}

func (s *Server) Start() {
	addr, err := net.ResolveTCPAddr("tcp", s.addr)
	if err != nil {
		panic(err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			continue
		}

		logger.Info("connection cccc")

		s.conn <- conn
	}
}
