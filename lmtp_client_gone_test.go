package smtp_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
)

// goneConn serves a whole pipelined session from memory; once gone is set,
// writes fail as they do to a peer that reset the connection.
type goneConn struct {
	closed    chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	in        *bytes.Reader
	gone      bool
	out       bytes.Buffer
}

var errPeerGone = errors.New("connection reset by peer")

func (c *goneConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.in.Read(p)
}

func (c *goneConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gone {
		return 0, errPeerGone
	}
	return c.out.Write(p)
}

func (c *goneConn) leave() { c.mu.Lock(); c.gone = true; c.mu.Unlock() }

func (c *goneConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *goneConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *goneConn) RemoteAddr() net.Addr             { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (c *goneConn) SetDeadline(time.Time) error      { return nil }
func (c *goneConn) SetReadDeadline(time.Time) error  { return nil }
func (c *goneConn) SetWriteDeadline(time.Time) error { return nil }

type oneConnListener struct {
	conn net.Conn
	done chan struct{}
	once sync.Once
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	var c net.Conn
	l.once.Do(func() { c = l.conn })
	if c != nil {
		return c, nil
	}
	<-l.done
	return nil, net.ErrClosed
}
func (l *oneConnListener) Close() error   { close(l.done); return nil }
func (l *oneConnListener) Addr() net.Addr { return &net.TCPAddr{} }

type countingBackend struct {
	mu        sync.Mutex
	delivered int
	onDeliver func(n int)
}

func (b *countingBackend) NewSession(*smtp.Conn) (smtp.Session, error) {
	return countingSession{b}, nil
}

type countingSession struct{ b *countingBackend }

func (s countingSession) Mail(string, *smtp.MailOptions) error { return nil }
func (s countingSession) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (s countingSession) Reset()                               {}
func (s countingSession) Logout() error                        { return nil }
func (s countingSession) Data(r io.Reader) error {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return err
	}
	s.b.mu.Lock()
	s.b.delivered++
	n := s.b.delivered
	s.b.mu.Unlock()
	if s.b.onDeliver != nil {
		s.b.onDeliver(n)
	}
	return nil
}

func pipelinedSession(n int) string {
	var b strings.Builder
	b.WriteString("LHLO client\r\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "MAIL FROM:<a@example.org>\r\nRCPT TO:<u@example.org>\r\nDATA\r\nSubject: %d\r\n\r\nbody\r\n.\r\n", i)
	}
	b.WriteString("QUIT\r\n")
	return b.String()
}

// TestLMTPClientGone: the client sends a batch and leaves while message
// leaveAt is being delivered; nothing after it may be delivered.
func TestLMTPClientGone(t *testing.T) {
	const batch = 20
	for _, tc := range []struct {
		name    string
		leaveAt int // 0 = the client reads every reply
		want    int
	}{
		{"client reads to the end", 0, batch},
		{"client leaves during the first message", 1, 1},
		{"client leaves mid batch", 7, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &goneConn{in: bytes.NewReader([]byte(pipelinedSession(batch))), closed: make(chan struct{})}
			be := &countingBackend{onDeliver: func(n int) {
				if n == tc.leaveAt {
					conn.leave()
				}
			}}
			s := smtp.NewServer(be)
			s.LMTP = true
			s.Domain = "localhost"
			s.ErrorLog = log.New(io.Discard, "", 0)
			go s.Serve(&oneConnListener{conn: conn, done: make(chan struct{})}) //nolint:errcheck
			<-conn.closed
			s.Close()
			be.mu.Lock()
			got := be.delivered
			be.mu.Unlock()
			if got != tc.want {
				t.Fatalf("delivered %d, want %d", got, tc.want)
			}
		})
	}
}
