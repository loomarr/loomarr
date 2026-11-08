package landiscovery

import (
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/platform/httpx"
)

const (
	BroadcastPort    = 51029
	BroadcastRequest = "LOOMARR_DISCOVER/1"
)

type broadcastResponse struct {
	Protocol int    `json:"protocol"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

type broadcastRegistration struct {
	conn net.PacketConn
	once sync.Once
	wg   sync.WaitGroup
}

func StartBroadcast(hostname string, publicURL func() string) (Registration, error) {
	registration, _, err := startBroadcastOn(net.JoinHostPort("", strconv.Itoa(BroadcastPort)), hostname, publicURL)
	return registration, err
}

func startBroadcastOn(address, hostname string, publicURL func() string) (Registration, net.Addr, error) {
	conn, err := net.ListenPacket("udp4", address)
	if err != nil {
		return nil, nil, err
	}
	name := instanceName(hostname)
	registration := &broadcastRegistration{conn: conn}
	registration.wg.Add(1)
	go registration.serve(name, "udp:"+name, publicURL)
	return registration, conn.LocalAddr(), nil
}

func (r *broadcastRegistration) serve(name, id string, publicURL func() string) {
	defer r.wg.Done()
	buf := make([]byte, 1025)
	for {
		n, sender, err := r.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		if n != len(BroadcastRequest) || string(buf[:n]) != BroadcastRequest || publicURL == nil ||
			!onLocalNetwork(sender) {
			continue
		}
		public := strings.TrimRight(strings.TrimSpace(publicURL()), "/")
		parsed, ok := httpURL(public)
		if !ok || localOnlyHost(parsed.Hostname()) {
			continue
		}
		payload, marshalErr := json.Marshal(broadcastResponse{Protocol: 1, ID: id, Name: name, URL: public})
		if marshalErr != nil {
			continue
		}
		_, _ = r.conn.WriteTo(payload, sender)
	}
}

// LocalOnlyURL reports whether an http(s) public URL names a host that no other device can reach:
// a loopback name or address, the unspecified address, or a link-local address. A lane backend's
// http://localhost:<port> is the usual case (#1892). Discovery stays silent rather than hand a
// phone or TV an address that leads back to itself.
func LocalOnlyURL(raw string) bool {
	parsed, ok := httpURL(strings.TrimSpace(raw))
	return ok && localOnlyHost(parsed.Hostname())
}

func httpURL(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, false
	}
	return parsed, true
}

func localOnlyHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast())
}

// onLocalNetwork reports whether a discovery request came from a device that could be on the
// household's network. The responder answers only those: a request from a public address is a
// stranger probing an exposed port, and the answer would hand them the server's address.
func onLocalNetwork(sender net.Addr) bool {
	udp, ok := sender.(*net.UDPAddr)
	return ok && !httpx.IsPublicIP(udp.IP)
}

func (r *broadcastRegistration) Shutdown() {
	if r == nil {
		return
	}
	r.once.Do(func() { _ = r.conn.Close() })
	r.wg.Wait()
}

func instanceName(hostname string) string {
	name := strings.TrimSuffix(strings.TrimSpace(hostname), ".local")
	if name == "" {
		name = "server"
	}
	return "Loomarr on " + name
}
