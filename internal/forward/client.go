package forward

import (
	"net"
	"time"

	"github.com/go-zoox/logger"
)

type Client struct {
	addr     string
	startAt  time.Time
	endAt    time.Time
	isClosed bool
}

func NewClient(addr string) *Client {
	return &Client{addr: addr}
}

func (c *Client) Connect(sourceConn *net.TCPConn) {
	targetConn, err := c.create()

	if err != nil {
		logger.Error("connect to %s err: %v", c.addr, err)
	}

	go Copy(sourceConn, targetConn, func() {

	})

	go Copy(targetConn, sourceConn, func() {
		c.Close()

		logger.Info("client close after %f s", c.GetAge().Seconds())
	})
}

func (c *Client) create() (*net.TCPConn, error) {
	c.startAt = time.Now()

	addr, err := net.ResolveTCPAddr("tcp", c.addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}

	// logger.Info("create tcp connection: %s", addr)
	return conn, nil
}

func (c *Client) Close() {
	c.endAt = time.Now()
	c.isClosed = true
}

func (c *Client) GetAge() time.Duration {
	return c.endAt.Sub(c.startAt)
}
