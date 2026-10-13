package ps_http //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"net"
	"net/http"

	"code.dny.dev/ssrf"
)

func SafeTransport() *http.Transport {
	safeGuardian := ssrf.New()
	dialer := &net.Dialer{
		Control: safeGuardian.Safe,
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	t := base.Clone()
	t.DialContext = dialer.DialContext
	return t
}
