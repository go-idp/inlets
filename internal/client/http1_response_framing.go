package client

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strconv"
)

// statusCodeFromResponseHead parses "HTTP/1.x NNN" from the first line of a raw response head.
func statusCodeFromResponseHead(head []byte) (code int) {
	lines := bytes.SplitN(head, []byte("\r\n"), 2)
	if len(lines) < 1 {
		return 0
	}
	parts := bytes.Split(bytes.TrimSpace(lines[0]), []byte(" "))
	if len(parts) < 2 {
		return 0
	}
	c, _ := strconv.Atoi(string(parts[1]))
	return c
}

// insertContentLength0IfUnframedEmpty adds "Content-Length: 0" when the head has neither
// Content-Length nor Transfer-Encoding, for HTTP/1.1+ messages with an empty body.
//
// Without this, a 401/403/407 with no framing headers and Connection: keep-alive leaves the
// message length undefined (RFC 7230); the browser can stay "pending" waiting for more bytes
// and may not run the auth challenge (WWW-Authenticate) heuristics correctly.
func insertContentLength0IfUnframedEmpty(head []byte) []byte {
	idx := bytes.Index(head, []byte("\r\n\r\n"))
	if idx < 0 {
		return head
	}
	hblock := head[:idx]
	lower := bytes.ToLower(hblock)
	if bytes.Contains(lower, []byte("\r\ncontent-length:")) {
		return head
	}
	if bytes.Contains(lower, []byte("\r\ntransfer-encoding:")) {
		return head
	}
	if lines := bytes.SplitN(hblock, []byte("\r\n"), 2); len(lines) < 1 {
		return head
	} else {
		tok := bytes.Fields(lines[0])
		if len(tok) > 0 && bytes.EqualFold(tok[0], []byte("HTTP/1.0")) {
			return head
		}
	}
	code := statusCodeFromResponseHead(head)
	if code == 0 {
		return head
	}
	switch code {
	case http.StatusContinue, 101, 102, 103: // 1xx: no message body
		return head
	case http.StatusNoContent, http.StatusNotModified: // 204, 304: no message body
		return head
	}
	insertion := []byte("Content-Length: 0")
	newHead := make([]byte, 0, len(head)+len(insertion)+2)
	newHead = append(newHead, head[:idx]...)
	newHead = append(newHead, "\r\n"...)
	newHead = append(newHead, insertion...)
	newHead = append(newHead, head[idx:]...)
	return newHead
}

// ensureHTTP1EmptyResponseFramed appends Content-Length: 0 when a fully-buffered, non-chunked
// response from net/http has an empty decompressed body and the wire head lacks content framing.
func ensureHTTP1EmptyResponseFramed(_ *http.Response, headFromDump []byte, bodySize int) []byte {
	if bodySize > 0 || headFromDump == nil {
		return headFromDump
	}
	return insertContentLength0IfUnframedEmpty(headFromDump)
}

// responseHeadForBufferedUpstreamBody re-serializes the status + headers for a response whose body
// has been fully read into `body` (e.g. de-chunked by net/http). It strips Transfer-Encoding and
// sets Content-Length to len(body) so the tunnel never forwards "chunked" headers while writing
// de-chunked bytes to the client — that mix makes curl/ingress wait forever for chunk terminators.
func responseHeadForBufferedUpstreamBody(resp *http.Response, body []byte) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil response")
	}
	nr := *resp
	nr.TransferEncoding = nil
	nr.Close = false
	nr.Request = nil
	nr.Header = resp.Header.Clone()
	nr.Header.Del("Trailer")
	nr.Header.Del("Transfer-Encoding")
	nr.Body = nil

	n := len(body)
	st := resp.StatusCode
	if (st >= 100 && st < 200) || st == http.StatusNoContent || st == http.StatusNotModified {
		if n != 0 {
			return nil, fmt.Errorf("unexpected %d body bytes for %d", n, st)
		}
		nr.ContentLength = -1
		nr.Header.Del("Content-Length")
	} else {
		nr.ContentLength = int64(n)
		if n > 0 {
			nr.Header.Set("Content-Length", strconv.Itoa(n))
		} else {
			nr.Header.Set("Content-Length", "0")
			// Empty message on a network where some intermediaries (e.g. ingress) still emit
			// Transfer-Encoding: chunked without a final chunk: force close so the end of the
			// response is unambiguous. Safe for the hijacked one-request/one-response path.
			nr.Close = true
			nr.Header.Del("Connection")
			nr.Header.Set("Connection", "close")
		}
	}

	b, err := httputil.DumpResponse(&nr, false)
	if err != nil {
		return nil, err
	}
	// If DumpResponse left TE or omitted framing, apply last-ditch fix for keep-alive 401, etc.
	if n == 0 && !((st >= 100 && st < 200) || st == http.StatusNoContent || st == http.StatusNotModified) {
		return insertContentLength0IfUnframedEmpty(b), nil
	}
	return b, nil
}
