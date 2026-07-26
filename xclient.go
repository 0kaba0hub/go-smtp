package smtp

import (
	"errors"
	"net"
	"strings"
)

// XClientData holds proxy information to convey to the relay via the XCLIENT
// command (Postfix extension: https://www.postfix.org/XCLIENT_README.html).
// Only non-zero fields are included in the command.
type XClientData struct {
	Addr  net.IP // originating client IP address
	HELO  string // originating client HELO/EHLO domain
	Login string // authenticated login name; empty = not authenticated
	Proto string // SMTP protocol variant: SMTP | ESMTP | ESMTPS | …
}

// XClient sends the XCLIENT command to the server, then re-negotiates EHLO.
// Must be called after the initial EHLO but before MAIL FROM.
//
// XCLIENT is a Postfix extension used by trusted front-ends to pass the real
// client IP/HELO to the relay so it logs and applies policy on the originating
// address rather than the submission server's address.
//
// After XCLIENT the server resets its SMTP state and responds with 220.
// This method consumes that response and re-sends EHLO automatically.
func (c *Client) XClient(data XClientData) error {
	if err := c.hello(); err != nil {
		return err
	}

	if _, ok := c.ext["XCLIENT"]; !ok {
		return errors.New("smtp: server does not support XCLIENT")
	}

	var params []string
	if data.Addr != nil {
		params = append(params, "ADDR="+data.Addr.String())
	}
	if data.HELO != "" {
		params = append(params, "HELO="+data.HELO)
	}
	if data.Login != "" {
		params = append(params, "LOGIN="+data.Login)
	}
	if data.Proto != "" {
		params = append(params, "PROTO="+data.Proto)
	}
	if len(params) == 0 {
		return errors.New("smtp: XClient: at least one parameter is required")
	}

	// Server responds with 220 (session-reset format) and clears SMTP state.
	if _, _, err := c.cmd(220, "XCLIENT %s", strings.Join(params, " ")); err != nil {
		return err
	}

	// Re-negotiate capabilities.
	// didGreet stays true — the initial greeting was already consumed;
	// XCLIENT's 220 was consumed by cmd() above.
	c.didHello = false
	c.helloError = nil
	c.ext = nil
	return c.hello()
}

// ---- server side (inbound XCLIENT, Postfix extension) -----------------------

// XClientAttrs holds the parameters of an INBOUND XCLIENT command received by
// the server from a trusted front-end proxy. The server performs no trust
// check — a Session implementing XClientReceiver validates the peer itself
// (Server.EnableXCLIENT must be set for the command to be accepted at all).
type XClientAttrs struct {
	Name  string // reverse-DNS name of the origin (NAME=)
	Addr  string // originating client IP (ADDR=; IPv6: prefix stripped)
	Port  string // originating client TCP port (PORT=)
	Proto string // protocol the origin used (PROTO=)
	Helo  string // origin HELO/EHLO domain (HELO=)
	Login string // authenticated login name at the origin (LOGIN=)
}

// XClientReceiver is implemented by a Session that wants to receive the
// parameters of an inbound XCLIENT command. It is invoked before the SMTP state
// is reset; the implementer decides whether the connection's peer is trusted.
type XClientReceiver interface {
	XClient(XClientAttrs)
}

// parseXClientAttrs parses a "KEY=VALUE KEY=VALUE" XCLIENT argument list. The
// [UNAVAILABLE] / [TEMPUNAVAIL] sentinels decode to an empty string; an
// "IPV6:" ADDR prefix (Postfix's spelling for IPv6 literals) is stripped.
func parseXClientAttrs(arg string) XClientAttrs {
	var a XClientAttrs
	for _, tok := range strings.Fields(arg) {
		eq := strings.IndexByte(tok, '=')
		if eq < 0 {
			continue
		}
		key, val := strings.ToUpper(tok[:eq]), tok[eq+1:]
		if strings.EqualFold(val, "[UNAVAILABLE]") || strings.EqualFold(val, "[TEMPUNAVAIL]") {
			val = ""
		}
		switch key {
		case "NAME":
			a.Name = val
		case "ADDR":
			if len(val) > 5 && strings.EqualFold(val[:5], "IPV6:") {
				val = val[5:]
			}
			a.Addr = val
		case "PORT":
			a.Port = val
		case "PROTO":
			a.Proto = val
		case "HELO":
			a.Helo = val
		case "LOGIN":
			a.Login = val
		}
	}
	return a
}

// handleXclient processes an inbound XCLIENT command (gated by
// Server.EnableXCLIENT). The parsed attributes are handed to the Session if it
// implements XClientReceiver, then — matching Postfix — the SMTP state is reset
// and a 220 greeting is sent so the client re-issues EHLO/LHLO.
func (c *Conn) handleXclient(arg string) {
	if !c.server.EnableXCLIENT {
		c.protocolError(500, EnhancedCode{5, 5, 1}, "XCLIENT unrecognized")
		return
	}
	attrs := parseXClientAttrs(arg)
	if r, ok := c.session.(XClientReceiver); ok {
		r.XClient(attrs)
	}
	c.reset()
	c.helo = "" // force a fresh EHLO/LHLO, as Postfix does after XCLIENT
	greet := "ESMTP"
	if c.server.LMTP {
		greet = "LMTP"
	}
	c.writeResponse(220, NoEnhancedCode, c.server.Domain+" "+greet)
}
