package smtp_test

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"

	smtp "github.com/emersion/go-smtp"
)

type xclientBackend struct{ got *smtp.XClientAttrs }

func (b *xclientBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &xclientSess{be: b}, nil
}

type xclientSess struct{ be *xclientBackend }

func (s *xclientSess) XClient(a smtp.XClientAttrs)          { v := a; s.be.got = &v }
func (s *xclientSess) Reset()                               {}
func (s *xclientSess) Logout() error                        { return nil }
func (s *xclientSess) Mail(string, *smtp.MailOptions) error { return nil }
func (s *xclientSess) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (s *xclientSess) Data(r io.Reader) error               { io.Copy(io.Discard, r); return nil }

func xclientTestConn(t *testing.T, enable bool) (*xclientBackend, net.Conn, *bufio.Scanner) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	be := &xclientBackend{}
	s := smtp.NewServer(be)
	s.Domain = "localhost"
	s.EnableXCLIENT = enable
	go s.Serve(l)
	c, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	sc := bufio.NewScanner(c)
	sc.Scan() // greeting
	return be, c, sc
}

func readEHLO(t *testing.T, c net.Conn, sc *bufio.Scanner) map[string]bool {
	t.Helper()
	io.WriteString(c, "EHLO relay\r\n")
	caps := map[string]bool{}
	for sc.Scan() {
		line := sc.Text()
		body := strings.TrimPrefix(strings.TrimPrefix(line, "250-"), "250 ")
		caps[body] = true
		if strings.HasPrefix(line, "250 ") {
			break
		}
	}
	return caps
}

func TestServerXClient_Enabled(t *testing.T) {
	be, c, sc := xclientTestConn(t, true)

	caps := readEHLO(t, c, sc)
	advertised := false
	for k := range caps {
		if strings.HasPrefix(k, "XCLIENT") {
			advertised = true
		}
	}
	if !advertised {
		t.Fatalf("EHLO must advertise XCLIENT when enabled, caps=%v", caps)
	}

	io.WriteString(c, "XCLIENT ADDR=203.0.113.9 PORT=54321 PROTO=ESMTP LOGIN=alice\r\n")
	if !sc.Scan() {
		t.Fatal("no reply to XCLIENT")
	}
	if !strings.HasPrefix(sc.Text(), "220") {
		t.Fatalf("XCLIENT reply must be 220 (reset), got %q", sc.Text())
	}
	if be.got == nil {
		t.Fatal("session did not receive XClient attrs")
	}
	if be.got.Addr != "203.0.113.9" || be.got.Port != "54321" || be.got.Login != "alice" {
		t.Fatalf("attrs = %+v, want Addr=203.0.113.9 Port=54321 Login=alice", *be.got)
	}
}

func TestServerXClient_DisabledUnrecognized(t *testing.T) {
	be, c, sc := xclientTestConn(t, false)

	caps := readEHLO(t, c, sc)
	for k := range caps {
		if strings.HasPrefix(k, "XCLIENT") {
			t.Fatalf("XCLIENT must not be advertised when disabled, caps=%v", caps)
		}
	}

	io.WriteString(c, "XCLIENT ADDR=203.0.113.9\r\n")
	if !sc.Scan() {
		t.Fatal("no reply")
	}
	if !strings.HasPrefix(sc.Text(), "500") {
		t.Fatalf("XCLIENT when disabled must be 500, got %q", sc.Text())
	}
	if be.got != nil {
		t.Fatal("session must not receive attrs when disabled")
	}
}

func TestServerXClient_IPv6AndUnavailable(t *testing.T) {
	be, c, sc := xclientTestConn(t, true)
	readEHLO(t, c, sc)
	io.WriteString(c, "XCLIENT ADDR=IPV6:2001:db8::1 NAME=[UNAVAILABLE]\r\n")
	sc.Scan()
	if be.got == nil || be.got.Addr != "2001:db8::1" || be.got.Name != "" {
		t.Fatalf("attrs = %+v, want Addr=2001:db8::1 Name empty", be.got)
	}
}
