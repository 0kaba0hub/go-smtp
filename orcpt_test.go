package smtp

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// ForceORCPT sends ORCPT, and only ORCPT, to a server that did not announce
// DSN; without it nothing DSN is sent there.
func TestClientForceORCPT(t *testing.T) {
	opts := &RcptOptions{OriginalRecipientType: DSNAddressTypeRFC822, OriginalRecipient: "first@example.com", Notify: []DSNNotify{DSNNotifyNever}}
	cases := []struct {
		name  string
		force bool
		want  string
	}{
		{"forced", true, "RCPT TO:<u@example.com> ORCPT=RFC822;first@example.com\r\n"},
		{"not forced", false, "RCPT TO:<u@example.com>\r\n"},
	}
	for _, c := range cases {
		var wrote bytes.Buffer
		var fake faker
		fake.ReadWriter = struct {
			io.Reader
			io.Writer
		}{strings.NewReader("220 hello\r\n250 ok\r\n"), &wrote}
		cl := NewClient(fake)
		cl.didHello = true
		cl.ext = map[string]string{}
		cl.ForceORCPT = c.force
		cl.Rcpt("u@example.com", opts)
		if got := wrote.String(); got != c.want {
			t.Errorf("%s: wrote %q, want %q", c.name, got, c.want)
		}
	}
}
