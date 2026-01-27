package forward

import (
	"io"
	"net"
	"strings"

	"github.com/go-zoox/logger"
)

func Copy(source *net.TCPConn, target *net.TCPConn, done func()) {
	for {
		data := make([]byte, 1024)
		n, err := source.Read(data)
		if err != nil {
			if err == io.EOF {
				logger.Info("source tcp closed: %v", err)
				source.Close()
			} else {
				logger.Info("read err: %v", err)
			}
			break
		}

		if _, err := target.Write(data[:n]); err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				logger.Info("target tcp closed(detail: %v), should close source force", err)
				// force source tcp conn close
				source.Close()

				done()
			} else {
				logger.Info("write err: %v", err)
			}

			break
		}
	}
}
