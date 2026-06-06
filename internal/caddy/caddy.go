package caddy

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const adminAPI = "http://localhost:2019"

type Manager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	dataDir string
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

// Start launches the Caddy process with an initial empty config
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	caddyBin, err := findCaddy()
	if err != nil {
		return fmt.Errorf("caddy not found: %w", err)
	}

	cfgPath := filepath.Join(m.dataDir, "caddy-initial.json")
	if err := os.WriteFile(cfgPath, []byte(initialConfig(m.dataDir)), 0600); err != nil {
		return err
	}

	m.cmd = exec.Command(caddyBin, "run", "--config", cfgPath, "--adapter", "")
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("start caddy: %w", err)
	}

	// Wait for admin API to be ready
	for i := 0; i < 20; i++ {
		resp, err := http.Get(adminAPI + "/config/")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("caddy admin API did not become ready")
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
}

// ProxyHost represents a single reverse proxy entry
type ProxyHost struct {
	ID              string
	Domain          string
	Upstream        string
	StripPath       bool
	WebSocket       bool
	ForceHTTPS      bool
	Enabled         bool
	MaintenanceMode bool
	ACMEEmail       string
	DNSProvider     string
	DNSConfig       string
	Staging         bool
}

type AccessRule struct {
	Type  string // basicauth | ip_allow | ip_deny
	Value string
}

// ApplyConfig pushes the full config to Caddy
func (m *Manager) ApplyConfig(hosts []ProxyHost, rules map[string][]AccessRule, dataDir string) error {
	cfg := buildConfig(hosts, rules, dataDir)
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return m.postConfig(b)
}

func (m *Manager) postConfig(body []byte) error {
	req, err := http.NewRequest("POST", adminAPI+"/load", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("caddy load: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy load failed %d: %s", resp.StatusCode, b)
	}
	return nil
}

func findCaddy() (string, error) {
	// Check common locations
	candidates := []string{"caddy", "/usr/bin/caddy", "/usr/local/bin/caddy", "/opt/caddy/caddy"}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("caddy binary not found in PATH or common locations")
}

func initialConfig(dataDir string) string {
	return fmt.Sprintf(`{
	"admin": {"listen": "localhost:2019"},
	"storage": {"module": "file_system", "root": "%s/caddy"},
	"apps": {
		"http": {"servers": {}},
		"tls": {}
	}
}`, dataDir)
}

// buildConfig constructs the full Caddy JSON config from proxy hosts
func buildConfig(hosts []ProxyHost, rules map[string][]AccessRule, dataDir string) map[string]interface{} {
	routes := []map[string]interface{}{}

	for _, h := range hosts {
		if !h.Enabled {
			continue
		}

		hostRules := rules[h.ID]
		route := buildRoute(h, hostRules)
		routes = append(routes, route...)
	}

	servers := map[string]interface{}{
		"srv0": map[string]interface{}{
			"listen": []string{":80", ":443"},
			"routes": routes,
		},
	}

	// Collect ACME emails for TLS automation
	acmeEmails := map[string]bool{}
	for _, h := range hosts {
		if h.Enabled && h.ACMEEmail != "" {
			acmeEmails[h.ACMEEmail] = true
		}
	}

	// Build per-domain TLS automation policies so Caddy knows exactly which domains to cert
	tlsPolicies := []map[string]interface{}{}
	for _, h := range hosts {
		if !h.Enabled || h.ACMEEmail == "" {
			continue
		}
		acmeDir := ""
		if h.Staging {
			acmeDir = "https://acme-staging-v02.api.letsencrypt.org/directory"
		}
		issuer := map[string]interface{}{
			"module": "acme",
			"email":  h.ACMEEmail,
		}
		if acmeDir != "" {
			issuer["ca"] = acmeDir
		}
		if h.DNSProvider != "" && h.DNSConfig != "" {
			issuer["challenges"] = map[string]interface{}{
				"dns": map[string]interface{}{
					"provider": h.DNSProvider,
				},
			}
		}
		policy := map[string]interface{}{
			"subjects": []string{h.Domain},
			"issuers":  []map[string]interface{}{issuer},
		}
		tlsPolicies = append(tlsPolicies, policy)
	}
	tlsCfg := map[string]interface{}{}
	if len(tlsPolicies) > 0 {
		tlsCfg["automation"] = map[string]interface{}{
			"policies": tlsPolicies,
		}
	}

	return map[string]interface{}{
		"admin": map[string]interface{}{"listen": "localhost:2019"},
		"storage": map[string]interface{}{
			"module": "file_system",
			"root":   dataDir + "/caddy",
		},
		"apps": map[string]interface{}{
			"http": map[string]interface{}{"servers": servers},
			"tls":  tlsCfg,
		},
	}
}

func buildRoute(h ProxyHost, rules []AccessRule) []map[string]interface{} {
	var routes []map[string]interface{}

	// Maintenance mode — serve a static page for all requests
	if h.MaintenanceMode {
		routes = append(routes, map[string]interface{}{
			"match": []map[string]interface{}{{"host": []string{h.Domain}}},
			"handle": []map[string]interface{}{
				{
					"handler":     "static_response",
					"status_code": 503,
					"headers": map[string]interface{}{
						"Content-Type":      []string{"text/html; charset=utf-8"},
						"Retry-After":       []string{"300"},
					},
					"body": maintenancePage(h.Domain),
				},
			},
		})
		return routes
	}

	// HTTP → HTTPS redirect
	if h.ForceHTTPS {
		routes = append(routes, map[string]interface{}{
			"match": []map[string]interface{}{
				{"host": []string{h.Domain}, "protocol": "http"},
			},
			"handle": []map[string]interface{}{
				{
					"handler":     "static_response",
					"status_code": 301,
					"headers": map[string]interface{}{
						"Location": []string{"https://{http.request.host}{http.request.uri}"},
					},
				},
			},
		})
	}

	// Build handlers chain
	handlers := []map[string]interface{}{}

	// IP deny rules
	denyIPs := []string{}
	allowIPs := []string{}
	for _, r := range rules {
		switch r.Type {
		case "ip_deny":
			denyIPs = append(denyIPs, r.Value)
		case "ip_allow":
			allowIPs = append(allowIPs, r.Value)
		}
	}

	if len(denyIPs) > 0 {
		handlers = append(handlers, map[string]interface{}{
			"handler": "subroute",
			"routes": []map[string]interface{}{
				{
					"match": []map[string]interface{}{
						{"remote_ip": map[string]interface{}{"ranges": denyIPs}},
					},
					"handle": []map[string]interface{}{
						{"handler": "static_response", "status_code": 403},
					},
				},
			},
		})
	}

	if len(allowIPs) > 0 {
		handlers = append(handlers, map[string]interface{}{
			"handler": "subroute",
			"routes": []map[string]interface{}{
				{
					"match": []map[string]interface{}{
						{
							"not": []map[string]interface{}{
								{"remote_ip": map[string]interface{}{"ranges": allowIPs}},
							},
						},
					},
					"handle": []map[string]interface{}{
						{"handler": "static_response", "status_code": 403},
					},
				},
			},
		})
	}

	// Basic auth rules
	basicAuthUsers := []map[string]interface{}{}
	for _, r := range rules {
		if r.Type == "basicauth" {
			// value format: "username:bcrypt_hash"
			parts := strings.SplitN(r.Value, ":", 2)
			if len(parts) == 2 {
				basicAuthUsers = append(basicAuthUsers, map[string]interface{}{
					"username": parts[0],
					"password": parts[1],
				})
			}
		}
	}
	if len(basicAuthUsers) > 0 {
		handlers = append(handlers, map[string]interface{}{
			"handler": "authentication",
			"providers": map[string]interface{}{
				"http_basic": map[string]interface{}{
					"accounts": basicAuthUsers,
					"hash": map[string]interface{}{
						"algorithm": "bcrypt",
					},
				},
			},
		})
	}

	// Reverse proxy handler
	upstream := h.Upstream
	if !strings.HasPrefix(upstream, "http://") && !strings.HasPrefix(upstream, "https://") {
		upstream = "http://" + upstream
	}

	rpHandler := map[string]interface{}{
		"handler": "reverse_proxy",
		"upstreams": []map[string]interface{}{
			{"dial": stripScheme(upstream)},
		},
		"headers": map[string]interface{}{
			"request": map[string]interface{}{
				"set": map[string][]string{
					"X-Forwarded-Proto": {"{http.request.scheme}"},
					"X-Real-IP":         {"{http.request.remote.host}"},
				},
			},
		},
	}

	if h.WebSocket {
		rpHandler["transport"] = map[string]interface{}{
			"protocol": "http",
			"versions": []string{"1.1", "2"},
		}
	}

	if h.StripPath {
		rpHandler["rewrite"] = map[string]interface{}{
			"strip_path_prefix": "/",
		}
	}

	handlers = append(handlers, rpHandler)

	mainRoute := map[string]interface{}{
		"match":  []map[string]interface{}{{"host": []string{h.Domain}}},
		"handle": handlers,
	}

	// TLS
	if h.ACMEEmail != "" {
		mainRoute["terminal"] = true
	}

	routes = append(routes, mainRoute)
	return routes
}

func stripScheme(u string) string {
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "https://")
	return u
}


func maintenancePage(domain string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Down for Maintenance</title>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;
    background:#0f1117;color:#e6edf3;display:flex;align-items:center;
    justify-content:center;min-height:100vh;padding:24px}
  .card{background:#161b22;border:1px solid #30363d;border-radius:12px;
    padding:48px;max-width:480px;width:100%;text-align:center}
  .icon{font-size:48px;margin-bottom:24px}
  h1{font-size:24px;font-weight:600;margin-bottom:12px}
  p{color:#8b949e;line-height:1.6;margin-bottom:8px}
  .domain{font-family:monospace;color:#2f81f7;font-size:13px}
</style>
</head>
<body>
<div class="card">
  <div class="icon">🔧</div>
  <h1>Down for Maintenance</h1>
  <p>This service is temporarily unavailable while we perform maintenance.</p>
  <p>Please check back shortly.</p>
  <p class="domain">` + domain + `</p>
</div>
</body>
</html>`
}

// GenerateSelfSigned creates a self-signed certificate for the given domain
func (m *Manager) GenerateSelfSigned(domain string, validDays int) (expiry time.Time, certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return
	}

	expiry = time.Now().Add(time.Duration(validDays) * 24 * time.Hour)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   domain,
			Organization: []string{"Haberdasher Self-Signed"},
		},
		DNSNames:  []string{domain},
		NotBefore: time.Now().Add(-1 * time.Minute),
		NotAfter:  expiry,
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

// StoreCert writes a cert/key pair to Caddy's storage directory
func (m *Manager) StoreCert(dataDir, domain string, certPEM, keyPEM []byte) error {
	dir := filepath.Join(dataDir, "caddy", "certificates", "local", domain)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, domain+".crt"), certPEM, 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, domain+".key"), keyPEM, 0600)
}

// ValidateAndStoreCert validates a user-supplied cert/key pair and stores it
func (m *Manager) ValidateAndStoreCert(dataDir, domain string, certPEMStr, keyPEMStr string) (time.Time, error) {
	certPEM := []byte(certPEMStr)
	keyPEM := []byte(keyPEMStr)

	// Parse and validate
	_, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return time.Time{}, fmt.Errorf("cert and key do not match: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, fmt.Errorf("invalid PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse certificate: %w", err)
	}

	if err := m.StoreCert(dataDir, domain, certPEM, keyPEM); err != nil {
		return time.Time{}, err
	}

	return cert.NotAfter, nil
}
