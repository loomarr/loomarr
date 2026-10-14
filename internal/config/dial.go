package config

import "net"

// DialableHostPort turns a LISTEN_ADDR into an address this same process can connect to. A wildcard
// bind (":8080", "0.0.0.0:8080", "[::]:8080") is an address to LISTEN on, not one to CONNECT to, so
// loopback is substituted while the port is kept. Used by the container healthcheck and by internal
// playout, which must reach its own server without going through server.public_url.
func DialableHostPort(listenAddr string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		// Not host:port at all (a bare port, or malformed). Treat the whole value as the port,
		// matching how net/http tolerates ":8080"-ish input, rather than failing over formatting.
		return net.JoinHostPort("127.0.0.1", listenAddr)
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
