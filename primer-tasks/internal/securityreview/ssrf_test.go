package securityreview

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
)

type fixtureResolver map[string][]net.IP

func (r fixtureResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ips, ok := r[host]
	if !ok {
		return nil, errors.New("not found")
	}
	out := make([]net.IPAddr, len(ips))
	for i, ip := range ips {
		out[i] = net.IPAddr{IP: ip}
	}
	return out, nil
}

func TestVerifierEndpointPolicy(t *testing.T) {
	resolver := fixtureResolver{
		"good.example.test":       {net.ParseIP("203.0.113.10")},
		"private.example.test":    {net.ParseIP("10.0.0.7")},
		"loopback.example.test":   {net.ParseIP("127.0.0.1")},
		"link-local.example.test": {net.ParseIP("169.254.10.3")},
		"metadata.example.test":   {net.ParseIP("169.254.169.254")},
		"rebind.example.test":     {net.ParseIP("203.0.113.10"), net.ParseIP("192.168.1.9")},
	}
	for name, raw := range map[string]string{
		"http": "http://good.example.test/verifier", "userinfo": "https://user:pass@good.example.test/verifier",
		"query":           "https://good.example.test/verifier?token=secret",
		"not allowlisted": "https://other.example.test/verifier", "private": "https://private.example.test/verifier",
		"loopback": "https://loopback.example.test/verifier", "link local": "https://link-local.example.test/verifier",
		"metadata": "https://metadata.example.test/verifier", "dns rebind": "https://rebind.example.test/verifier",
	} {
		_, err := ValidateVerifierEndpoint(context.Background(), raw, []string{"good.example.test", "private.example.test", "loopback.example.test", "link-local.example.test", "metadata.example.test", "rebind.example.test"}, resolver)
		if err == nil {
			t.Errorf("%s endpoint accepted: %s", name, raw)
		}
	}
	if target, err := ValidateVerifierEndpoint(context.Background(), "https://good.example.test/verifier", []string{"good.example.test"}, resolver); err != nil || target.Host != "good.example.test" || len(target.IPs) != 1 {
		t.Fatalf("good endpoint = %+v, %v", target, err)
	}
	if _, err := ValidateVerifierEndpoint(context.Background(), "https://good.example.test/verifier", nil, resolver); err == nil {
		t.Fatal("empty allowlist accepted")
	}
}

func TestVerifierPolicyBlocksLiteralSpecialAddressesAndWildcardBoundary(t *testing.T) {
	resolver := fixtureResolver{}
	for _, host := range []string{"localhost", "127.0.0.1", "169.254.169.254", "192.168.1.1", "10.0.0.1", "::1", "metadata.google.internal"} {
		_, err := ValidateVerifierEndpoint(context.Background(), "https://"+host+"/callback", []string{host, "*.example.test"}, resolver)
		if err == nil {
			t.Fatalf("special address accepted: %s", host)
		}
	}
	resolver["verifier.example.test"] = []net.IP{net.ParseIP("203.0.113.20")}
	if _, err := ValidateVerifierEndpoint(context.Background(), "https://verifier.example.test/callback", []string{"*.example.test"}, resolver); err != nil {
		t.Fatal(err)
	}
	resolver["example.test"] = []net.IP{net.ParseIP("203.0.113.20")}
	if _, err := ValidateVerifierEndpoint(context.Background(), "https://example.test/callback", []string{"*.example.test"}, resolver); err == nil {
		t.Fatal("wildcard allowlist matched its parent")
	}
}

func TestDNSRebindingIsRejectedBeforeCredentialedConnect(t *testing.T) {
	resolver := fixtureResolver{"good.example.test": {net.ParseIP("203.0.113.10")}}
	target, err := ValidateVerifierEndpoint(context.Background(), "https://good.example.test/callback", []string{"good.example.test"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	resolver["good.example.test"] = []net.IP{net.ParseIP("203.0.113.11")}
	if err := RevalidatePinnedEndpoint(context.Background(), target, resolver); err == nil {
		t.Fatal("changed DNS answer accepted")
	}
	resolver["good.example.test"] = []net.IP{net.ParseIP("203.0.113.10"), net.ParseIP("10.0.0.2")}
	if err := RevalidatePinnedEndpoint(context.Background(), target, resolver); err == nil {
		t.Fatal("private rebinding answer accepted")
	}
}

func TestPinnedTransportRevalidatesBeforeDial(t *testing.T) {
	resolver := fixtureResolver{"good.example.test": {net.ParseIP("203.0.113.10")}}
	target, err := ValidateVerifierEndpoint(context.Background(), "https://good.example.test/callback", []string{"good.example.test"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := PinnedTransport(context.Background(), target, resolver).(*http.Transport)
	if !ok || transport.TLSClientConfig.ServerName != target.Host {
		t.Fatalf("transport=%T config=%v", transport, transport.TLSClientConfig)
	}
	resolver["good.example.test"] = []net.IP{net.ParseIP("192.168.1.2")}
	if _, err := transport.DialContext(context.Background(), "tcp", "good.example.test:443"); err == nil {
		t.Fatal("rebinding transport dial succeeded")
	}
}

func TestPinnedTransportUsesDefaultResolverWhenNoneIsProvided(t *testing.T) {
	resolver := fixtureResolver{"good.example.test": {net.ParseIP("203.0.113.10")}}
	target, err := ValidateVerifierEndpoint(context.Background(), "https://good.example.test/callback", []string{"good.example.test"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := PinnedTransport(context.Background(), target, nil).(*http.Transport)
	if !ok || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("default-resolver transport=%T config=%v", transport, transport.TLSClientConfig)
	}
}

func TestPinnedTransportReportsUnreachableApprovedAddresses(t *testing.T) {
	resolver := fixtureResolver{"good.example.test": {net.ParseIP("203.0.113.10")}}
	target, err := ValidateVerifierEndpoint(context.Background(), "https://good.example.test/callback", []string{"good.example.test"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	transport := PinnedTransport(context.Background(), target, resolver).(*http.Transport)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := transport.DialContext(ctx, "tcp", "good.example.test:443"); err == nil {
		t.Fatal("canceled unreachable dial succeeded")
	}
}

func TestRedirectAndCredentialForwardingAreBoundaries(t *testing.T) {
	if err := NoRedirect(&http.Request{URL: &url.URL{Scheme: "https", Host: "other.example.test"}}, nil); err == nil {
		t.Fatal("redirect accepted")
	}
	resolver := fixtureResolver{"good.example.test": {net.ParseIP("203.0.113.10")}}
	target, err := ValidateVerifierEndpoint(context.Background(), "https://good.example.test/callback", []string{"good.example.test"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := CredentialHeader(target, []string{"good.example.test"}, "opaque-secret")
	if err != nil || headers.Get("Authorization") != "Bearer opaque-secret" {
		t.Fatalf("approved credential = %v, %v", headers, err)
	}
	if _, err := CredentialHeader(target, []string{"other.example.test"}, "opaque-secret"); err == nil {
		t.Fatal("credential forwarded to a different allowlist")
	}
	if _, err := CredentialHeader(target, []string{"good.example.test"}, ""); err == nil {
		t.Fatal("empty credential forwarded")
	}
}
