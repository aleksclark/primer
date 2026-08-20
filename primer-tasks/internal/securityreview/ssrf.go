package securityreview

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// IPResolver is injected by policy tests and by the production egress dialer.
// Resolving before connecting, then dialing only the pinned answers, closes the
// DNS-rebinding gap between validation and use.
type IPResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}
type NetResolver struct{}

func (NetResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

type EgressTarget struct {
	URL  *url.URL
	Host string
	IPs  []net.IP
}

func ValidateVerifierEndpoint(ctx context.Context, raw string, allowlist []string, resolver IPResolver) (EgressTarget, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return EgressTarget{}, errors.New("verifier endpoint must be HTTPS without credentials, query, or fragment")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if !allowedHost(host, allowlist) {
		return EgressTarget{}, fmt.Errorf("verifier host %q is not allowlisted", host)
	}
	if blockedHostname(host) {
		return EgressTarget{}, errors.New("verifier host is private, loopback, link-local, or metadata")
	}
	if resolver == nil {
		resolver = NetResolver{}
	}
	answers, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(answers) == 0 {
		return EgressTarget{}, errors.New("verifier hostname did not resolve")
	}
	ips := make([]net.IP, 0, len(answers))
	for _, answer := range answers {
		ip := answer.IP
		if blockedIP(ip) {
			return EgressTarget{}, fmt.Errorf("verifier resolved to forbidden address %s", ip)
		}
		ips = append(ips, append(net.IP(nil), ip...))
	}
	return EgressTarget{URL: u, Host: host, IPs: ips}, nil
}

// RevalidatePinnedEndpoint is called immediately before each connection. DNS
// answers must remain byte-for-byte the set approved at catalog validation;
// changing an answer is a DNS-rebinding failure, not a reason to trust the new
// address.
func RevalidatePinnedEndpoint(ctx context.Context, target EgressTarget, resolver IPResolver) error {
	if resolver == nil {
		resolver = NetResolver{}
	}
	answers, err := resolver.LookupIPAddr(ctx, target.Host)
	if err != nil || len(answers) != len(target.IPs) {
		return errors.New("verifier DNS answers changed")
	}
	seen := make(map[string]bool, len(answers))
	for _, answer := range answers {
		if blockedIP(answer.IP) {
			return errors.New("verifier DNS answer is forbidden")
		}
		seen[answer.IP.String()] = true
	}
	for _, ip := range target.IPs {
		if !seen[ip.String()] {
			return errors.New("verifier DNS answers changed")
		}
	}
	return nil
}

func allowedHost(host string, allowlist []string) bool {
	for _, raw := range allowlist {
		entry := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if entry == host || (strings.HasPrefix(entry, "*.") && strings.HasSuffix(host, entry[1:]) && host != entry[2:]) {
			return true
		}
	}
	return false
}
func blockedHostname(host string) bool {
	if host == "localhost" || host == "metadata" || host == "metadata.google.internal" || host == "instance-data" || host == "instance-data.ec2.internal" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return blockedIP(ip)
	}
	return false
}
func blockedIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.ParseIP("169.254.169.254")) || ip.Equal(net.ParseIP("fd00:ec2::254"))
}

// NoRedirect is mandatory for verifier clients. A redirect changes the
// authority after the allowlist and pinned-resolution checks and must fail.
func NoRedirect(_ *http.Request, _ []*http.Request) error {
	return errors.New("verifier redirects are forbidden")
}

// CredentialHeader is intentionally separate from URL validation: credentials
// are produced only after a target has passed the same exact allowlist policy.
// Callers must not copy this header into a redirect or a different host.
func CredentialHeader(target EgressTarget, allowlist []string, credential string) (http.Header, error) {
	if target.URL == nil || target.URL.Scheme != "https" || !allowedHost(target.Host, allowlist) || credential == "" {
		return nil, errors.New("credential forwarding target is not approved")
	}
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+credential)
	return h, nil
}
