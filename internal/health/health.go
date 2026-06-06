package health

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/haberdasher/haberdasher/internal/db"
)

type Checker struct {
	db *db.DB
}

func NewChecker(database *db.DB) *Checker {
	return &Checker{db: database}
}

func (c *Checker) Start() {
	log.Println("[health] checker started")
	go c.loop()
}

func (c *Checker) loop() {
	for {
		hosts, err := c.db.ListProxyHosts()
		if err != nil {
			log.Printf("[health] list hosts error: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		now := time.Now()
		for _, h := range hosts {
			if !h.Enabled {
				continue
			}

			// Frontend/TLS check interval
			frontendAge := now.Sub(h.HealthCheckedAt)
			frontendInterval := time.Duration(60 * time.Second)
			if h.CertStatus == "pending" || h.CertStatus == "none" || h.CertStatus == "" {
				frontendInterval = 10 * time.Second
			} else if h.HealthStatus == "down" {
				frontendInterval = 30 * time.Second
			}

			// Upstream check interval
			upstreamAge := now.Sub(h.UpstreamCheckedAt)
			upstreamInterval := time.Duration(30 * time.Second)
			if h.UpstreamStatus == "down" {
				upstreamInterval = 15 * time.Second
			}

			if frontendAge >= frontendInterval {
				go c.checkFrontend(h)
			}
			if upstreamAge >= upstreamInterval {
				go c.checkUpstream(h)
			}
		}

		time.Sleep(5 * time.Second)
	}
}

// checkFrontend hits 127.0.0.1:443 with the correct SNI/Host header.
// Avoids hairpin NAT — connects directly to Caddy inside the container.
func (c *Checker) checkFrontend(h *db.ProxyHost) {
	log.Printf("[health] frontend check: %s", h.Domain)
	start := time.Now()

	// Skip maintenance mode hosts — a 503 from our own maintenance page isn't "down"
	if h.MaintenanceMode {
		log.Printf("[health] %s skipping frontend check — maintenance mode active", h.Domain)
		fresh, dbErr := c.db.GetProxyHost(h.ID)
		if dbErr == nil && fresh != nil {
			fresh.HealthStatus = "maintenance"
			fresh.HealthCheckedAt = time.Now()
			fresh.UpdatedAt = time.Now()
			c.db.UpdateProxyHost(fresh)
		}
		return
	}

	// Self-signed and staging certs are not trusted by system CA — skip TLS verify
	// so we can still connect and read cert info without false "down" reports
	insecure := false
	if cert, _ := c.db.GetCertByHostID(h.ID); cert != nil {
		insecure = cert.Provider == "selfsigned" || cert.Staging
	}

	tlsCfg := &tls.Config{
		ServerName:         h.Domain,
		InsecureSkipVerify: insecure,
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			TLSClientConfig:       tlsCfg,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			// Force connection to local Caddy regardless of DNS
			DialTLS: func(network, addr string) (net.Conn, error) {
				return tls.Dial(network, "127.0.0.1:443", &tls.Config{
					ServerName:         h.Domain,
					InsecureSkipVerify: insecure,
				})
			},
		},
	}

	req, err := http.NewRequest("GET", "https://"+h.Domain, nil)
	if err != nil {
		log.Printf("[health] %s frontend request error: %v", h.Domain, err)
		return
	}

	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	// Read fresh copy to avoid race with upstream checker
	fresh, dbErr := c.db.GetProxyHost(h.ID)
	if dbErr != nil || fresh == nil {
		return
	}

	fresh.HealthCheckedAt = time.Now()
	fresh.UpdatedAt = time.Now()

	if err != nil {
		log.Printf("[health] %s frontend DOWN: %v", h.Domain, err)
		fresh.HealthStatus = "down"
		fresh.HealthLatencyMs = 0
	} else {
		defer resp.Body.Close()
		fresh.HealthLatencyMs = int(latency)

		if resp.StatusCode >= 500 {
			log.Printf("[health] %s frontend DOWN (Caddy error %d)", h.Domain, resp.StatusCode)
			fresh.HealthStatus = "down"
		} else {
			fresh.HealthStatus = "up"
			log.Printf("[health] %s frontend UP %dms (status %d)", h.Domain, latency, resp.StatusCode)
		}

		// Read cert from TLS handshake
		if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
			leaf := resp.TLS.PeerCertificates[0]
			days := int(time.Until(leaf.NotAfter).Hours() / 24)
			log.Printf("[health] %s cert valid until %s (%d days)", h.Domain, leaf.NotAfter.Format("2006-01-02"), days)
			fresh.CertStatus = "active"
			fresh.CertExpiry = leaf.NotAfter
			fresh.CertExpiryUnix = leaf.NotAfter.Unix()

			// Sync to certificates table too
			if dbCert, _ := c.db.GetCertByHostID(h.ID); dbCert != nil {
				dbCert.Status = "active"
				dbCert.Expiry = leaf.NotAfter
				dbCert.UpdatedAt = time.Now()
				c.db.UpsertCertificate(dbCert)
			}
		}
	}

	if err := c.db.UpdateProxyHost(fresh); err != nil {
		log.Printf("[health] update host %s: %v", h.Domain, err)
	}
}

// checkUpstream hits the upstream directly (e.g. 192.168.1.5:8080) via plain HTTP.
func (c *Checker) checkUpstream(h *db.ProxyHost) {
	upstream := h.Upstream
	if upstream == "" {
		return
	}

	// Build URL — add http:// if no scheme
	upstreamURL := upstream
	if !strings.HasPrefix(upstreamURL, "http://") && !strings.HasPrefix(upstreamURL, "https://") {
		upstreamURL = "http://" + upstream
	}

	log.Printf("[health] upstream check: %s -> %s", h.Domain, upstreamURL)
	start := time.Now()

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(upstreamURL)
	latency := time.Since(start).Milliseconds()

	fresh, dbErr := c.db.GetProxyHost(h.ID)
	if dbErr != nil || fresh == nil {
		return
	}

	fresh.UpstreamCheckedAt = time.Now()
	fresh.UpdatedAt = time.Now()

	if err != nil {
		log.Printf("[health] %s upstream DOWN: %v", h.Domain, err)
		fresh.UpstreamStatus = "down"
		fresh.UpstreamLatencyMs = 0
	} else {
		defer resp.Body.Close()
		fresh.UpstreamLatencyMs = int(latency)
		if resp.StatusCode >= 500 {
			log.Printf("[health] %s upstream DOWN (server error %d)", h.Domain, resp.StatusCode)
			fresh.UpstreamStatus = "down"
		} else {
			fresh.UpstreamStatus = "up"
			log.Printf("[health] %s upstream UP %dms (status %d)", h.Domain, latency, resp.StatusCode)
		}
	}

	if err := c.db.UpdateProxyHost(fresh); err != nil {
		log.Printf("[health] update upstream %s: %v", h.Domain, err)
	}
}

// CheckNow forces an immediate check on both frontend and upstream
func (c *Checker) CheckNow(h *db.ProxyHost) {
	log.Printf("[health] immediate check requested for %s", h.Domain)
	go c.checkFrontend(h)
	go c.checkUpstream(h)
}

type StatusSummary struct {
	Up      int
	Down    int
	Unknown int
}

func FormatExpiry(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	days := int(time.Until(t).Hours() / 24)
	switch {
	case days < 0:
		return fmt.Sprintf("expired %dd ago", -days)
	case days == 0:
		return "expires today"
	default:
		return fmt.Sprintf("%d days", days)
	}
}
