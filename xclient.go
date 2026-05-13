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
