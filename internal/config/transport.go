package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func (c Config) validateTransport() error {
	if c.TransportMode != "development" && c.TransportMode != "production" {
		return fmt.Errorf("TRANSPORT_MODE must be production or development")
	}
	if c.TransportMode != "production" {
		return nil
	}
	host, _, err := net.SplitHostPort(c.Address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("production SERVER_ADDRESS must bind a loopback IP behind a same-host HTTPS proxy/tunnel")
	}
	u, err := url.Parse(c.PublicBaseURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("production PUBLIC_BASE_URL must use HTTPS without credentials, query or fragment")
	}
	if len(c.FrontendOrigins) == 0 {
		return fmt.Errorf("production FRONTEND_ORIGINS must list explicit trusted frontend hosts")
	}
	for _, origin := range c.FrontendOrigins {
		// Retain the existing host[:port] format used by coder/websocket.
		u, err := url.Parse("https://" + origin)
		if err != nil || u.Host != origin || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(origin, "*?\\") {
			return fmt.Errorf("production FRONTEND_ORIGINS requires exact host[:port] entries; no scheme, wildcard, credentials or path")
		}
	}
	return nil
}
