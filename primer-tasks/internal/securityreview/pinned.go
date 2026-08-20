package securityreview

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strconv"
)

// PinnedTransport rechecks DNS immediately before each connection and dials
// only the approved answers. TLS still uses the original verifier hostname,
// so the certificate authority is never replaced by an IP literal.
func PinnedTransport(ctx context.Context, target EgressTarget, resolver IPResolver) http.RoundTripper {
	resolver = func() IPResolver {
		if resolver == nil {
			return NetResolver{}
		}
		return resolver
	}()
	return &http.Transport{
		TLSClientConfig: &tls.Config{ServerName: target.Host, MinVersion: tls.VersionTLS12},
		DialContext: func(dialCtx context.Context, network, _ string) (net.Conn, error) {
			if err := RevalidatePinnedEndpoint(dialCtx, target, resolver); err != nil {
				return nil, err
			}
			port := target.URL.Port()
			if port == "" {
				port = strconv.Itoa(443)
			}
			for _, ip := range target.IPs {
				conn, err := (&net.Dialer{}).DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
			}
			return nil, errors.New("approved verifier addresses are unreachable")
		},
		ForceAttemptHTTP2: true,
	}
}
