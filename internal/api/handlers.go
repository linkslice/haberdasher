package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/haberdasher/haberdasher/internal/auth"
	caddymgr "github.com/haberdasher/haberdasher/internal/caddy"
	"github.com/haberdasher/haberdasher/internal/db"
	"github.com/haberdasher/haberdasher/internal/health"
	"github.com/haberdasher/haberdasher/internal/metrics"
)

type contextKey string

const claimsKey contextKey = "claims"

type Server struct {
	checker *health.Checker
	db      *db.DB
	caddy   *caddymgr.Manager
	bus     *metrics.Bus
	jwtSec  string
	dataDir string
}

func NewServer(database *db.DB, caddy *caddymgr.Manager, bus *metrics.Bus, checker *health.Checker, jwtSecret, dataDir string) *Server {
	return &Server{db: database, caddy: caddy, bus: bus, checker: checker, jwtSec: jwtSecret, dataDir: dataDir}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	r.Get("/api/setup/status", s.setupStatus)
	r.Post("/api/setup/complete", s.setupComplete)
	r.Post("/api/auth/login", s.login)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)

		r.Get("/api/auth/me", s.me)
		r.Post("/api/auth/change-password", s.changePassword)
		r.Post("/api/auth/totp/setup", s.totpSetup)
		r.Post("/api/auth/totp/confirm", s.totpConfirm)
		r.Delete("/api/auth/totp", s.totpDisable)

		r.Get("/api/hosts", s.listHosts)
		r.Post("/api/hosts", s.createHost)
		r.Get("/api/hosts/{id}", s.getHost)
		r.Put("/api/hosts/{id}", s.updateHost)
		r.Delete("/api/hosts/{id}", s.deleteHost)
		r.Post("/api/hosts/{id}/toggle", s.toggleHost)
		r.Post("/api/hosts/{id}/maintenance", s.toggleMaintenance)

		r.Get("/api/hosts/{id}/rules", s.listRules)
		r.Post("/api/hosts/{id}/rules", s.createRule)
		r.Delete("/api/hosts/{id}/rules/{ruleID}", s.deleteRule)

		r.Get("/api/certificates", s.listCerts)
		r.Post("/api/hosts/{id}/certificate", s.requestCert)
		r.Get("/api/hosts/{id}/certificate", s.getCert)
		r.Delete("/api/certificates/{id}", s.deleteCert)

		r.Get("/api/monitoring/destinations", s.listDestinations)
		r.Post("/api/monitoring/destinations", s.createDestination)
		r.Put("/api/monitoring/destinations/{id}", s.updateDestination)
		r.Delete("/api/monitoring/destinations/{id}", s.deleteDestination)
		r.Post("/api/monitoring/destinations/{id}/test", s.testDestination)
		r.Post("/api/monitoring/destinations/{id}/toggle", s.toggleDestination)

		r.Get("/api/settings", s.getSettings)
		r.Put("/api/settings", s.updateSettings)

		r.Get("/api/audit", s.auditLog)
	})

	return r
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token := strings.TrimPrefix(header, "Bearer ")
		if token == "" {
			if c, err := r.Cookie("haber_token"); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			errJSON(w, 401, "unauthorized")
			return
		}
		claims, err := auth.ValidateJWT(s.jwtSec, token)
		if err != nil {
			errJSON(w, 401, "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) claimsFromRequest(r *http.Request) *auth.Claims {
	v := r.Context().Value(claimsKey)
	if v == nil {
		return &auth.Claims{}
	}
	return v.(*auth.Claims)
}

func (s *Server) reloadCaddy() error {
	hosts, err := s.db.ListProxyHosts()
	if err != nil {
		return err
	}
	caddyHosts := []caddymgr.ProxyHost{}
	rulesMap := map[string][]caddymgr.AccessRule{}
	for _, h := range hosts {
		cert, _ := s.db.GetCertByHostID(h.ID)
		ch := caddymgr.ProxyHost{
			ID:              h.ID,
			Domain:          h.Domain,
			Upstream:        h.Upstream,
			StripPath:       h.StripPath,
			WebSocket:       h.WebSocket,
			ForceHTTPS:      h.ForceHTTPS,
			Enabled:         h.Enabled,
			MaintenanceMode: h.MaintenanceMode,
		}
		if cert != nil {
			ch.ACMEEmail = cert.ACMEEmail
			ch.DNSProvider = cert.DNSProvider
			ch.DNSConfig = cert.DNSConfig
			ch.Staging = cert.Staging
		}
		caddyHosts = append(caddyHosts, ch)
		dbRules, _ := s.db.ListAccessRules(h.ID)
		for _, r := range dbRules {
			rulesMap[h.ID] = append(rulesMap[h.ID], caddymgr.AccessRule{Type: r.Type, Value: r.Value})
		}
	}
	return s.caddy.ApplyConfig(caddyHosts, rulesMap, s.dataDir)
}

func (s *Server) reloadMetrics() {
	dests, err := s.db.ListMetricDestinations()
	if err != nil {
		return
	}
	md := make([]metrics.Destination, len(dests))
	for i, d := range dests {
		md[i] = metrics.Destination{
			ID: d.ID, Name: d.Name, Type: d.Type,
			Host: d.Host, Port: d.Port, Prefix: d.Prefix,
			TLS: d.TLS, ConfigJSON: d.ConfigJSON, Enabled: d.Enabled,
		}
	}
	s.bus.SetDestinations(md)
}

// ── Setup ─────────────────────────────────────────────────────────────────────

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	done, _ := s.db.IsSetupComplete()
	writeJSON(w, 200, map[string]bool{"complete": done})
}

func (s *Server) setupComplete(w http.ResponseWriter, r *http.Request) {
	done, _ := s.db.IsSetupComplete()
	if done {
		errJSON(w, 400, "setup already complete")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil || req.Email == "" || req.Password == "" {
		errJSON(w, 400, "email and password required")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	user := &db.User{
		ID: uuid.New().String(), Email: req.Email,
		PasswordHash: hash, Role: "admin", CreatedAt: time.Now(),
	}
	if err := s.db.CreateUser(user); err != nil {
		errJSON(w, 500, "create user failed")
		return
	}
	s.db.SetSetting("setup_complete", "true")
	s.db.SetSetting("admin_email", req.Email)
	tok, _ := auth.GenerateJWT(s.jwtSec, user.ID, user.Email, user.Role)
	writeJSON(w, 200, map[string]string{"token": tok})
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		TOTPCode string `json:"totp_code"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, "bad request")
		return
	}
	user, err := s.db.GetUserByEmail(req.Email)
	if err != nil || user == nil {
		errJSON(w, 401, "invalid credentials")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		errJSON(w, 401, "invalid credentials")
		return
	}
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			writeJSON(w, 200, map[string]interface{}{"totp_required": true})
			return
		}
		if !auth.ValidateTOTP(user.TOTPSecret, req.TOTPCode) {
			errJSON(w, 401, "invalid TOTP code")
			return
		}
	}
	tok, _ := auth.GenerateJWT(s.jwtSec, user.ID, user.Email, user.Role)
	writeJSON(w, 200, map[string]string{"token": tok})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	user, _ := s.db.GetUserByID(claims.UserID)
	if user == nil {
		errJSON(w, 404, "user not found")
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"id": user.ID, "email": user.Email,
		"role": user.Role, "totp_enabled": user.TOTPEnabled,
	})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	readJSON(r, &req)
	user, _ := s.db.GetUserByID(claims.UserID)
	if user == nil || !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		errJSON(w, 401, "incorrect current password")
		return
	}
	hash, _ := auth.HashPassword(req.NewPassword)
	s.db.UpdateUserPassword(user.ID, hash)
	s.db.AuditLog(claims.UserID, "password_changed", "user:"+user.ID, "")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	user, _ := s.db.GetUserByID(claims.UserID)
	secret, url, err := auth.GenerateTOTPSecret(user.Email)
	if err != nil {
		errJSON(w, 500, "totp generate failed")
		return
	}
	s.db.UpdateUserTOTP(user.ID, secret, false)
	writeJSON(w, 200, map[string]string{"secret": secret, "otpauth_url": url})
}

func (s *Server) totpConfirm(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	var req struct {
		Code string `json:"code"`
	}
	readJSON(r, &req)
	user, _ := s.db.GetUserByID(claims.UserID)
	if !auth.ValidateTOTP(user.TOTPSecret, req.Code) {
		errJSON(w, 400, "invalid code")
		return
	}
	s.db.UpdateUserTOTP(user.ID, user.TOTPSecret, true)
	s.db.AuditLog(claims.UserID, "totp_enabled", "user:"+user.ID, "")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) totpDisable(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	s.db.UpdateUserTOTP(claims.UserID, "", false)
	s.db.AuditLog(claims.UserID, "totp_disabled", "user:"+claims.UserID, "")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ── Proxy Hosts ───────────────────────────────────────────────────────────────

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.db.ListProxyHosts()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if hosts == nil {
		hosts = []*db.ProxyHost{}
	}
	writeJSON(w, 200, hosts)
}

func (s *Server) createHost(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	var req struct {
		Domain       string `json:"domain"`
		Upstream     string `json:"upstream"`
		MetricsAlias string `json:"metrics_alias"`
		StripPath    bool   `json:"strip_path"`
		WebSocket    bool   `json:"websocket"`
		ForceHTTPS   bool   `json:"force_https"`
	}
	if err := readJSON(r, &req); err != nil || req.Domain == "" || req.Upstream == "" {
		errJSON(w, 400, "domain and upstream required")
		return
	}
	h := &db.ProxyHost{
		ID: uuid.New().String(), Domain: req.Domain, Upstream: req.Upstream,
		MetricsAlias: req.MetricsAlias, StripPath: req.StripPath,
		WebSocket: req.WebSocket, ForceHTTPS: req.ForceHTTPS,
		Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := s.db.CreateProxyHost(h); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	s.db.AuditLog(claims.UserID, "host_created", "host:"+h.ID, h.Domain)
	s.reloadCaddy()
	writeJSON(w, 201, h)
}

func (s *Server) getHost(w http.ResponseWriter, r *http.Request) {
	h, err := s.db.GetProxyHost(chi.URLParam(r, "id"))
	if err != nil || h == nil {
		errJSON(w, 404, "not found")
		return
	}
	writeJSON(w, 200, h)
}

func (s *Server) updateHost(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	id := chi.URLParam(r, "id")
	h, err := s.db.GetProxyHost(id)
	if err != nil || h == nil {
		errJSON(w, 404, "not found")
		return
	}
	var req struct {
		Domain       string `json:"domain"`
		Upstream     string `json:"upstream"`
		MetricsAlias string `json:"metrics_alias"`
		StripPath    bool   `json:"strip_path"`
		WebSocket    bool   `json:"websocket"`
		ForceHTTPS   bool   `json:"force_https"`
	}
	readJSON(r, &req)
	h.Domain = req.Domain
	h.Upstream = req.Upstream
	h.MetricsAlias = req.MetricsAlias
	h.StripPath = req.StripPath
	h.WebSocket = req.WebSocket
	h.ForceHTTPS = req.ForceHTTPS
	h.UpdatedAt = time.Now()
	s.db.UpdateProxyHost(h)
	s.db.AuditLog(claims.UserID, "host_updated", "host:"+h.ID, h.Domain)
	s.reloadCaddy()
	writeJSON(w, 200, h)
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	id := chi.URLParam(r, "id")
	s.db.DeleteProxyHost(id)
	s.db.AuditLog(claims.UserID, "host_deleted", "host:"+id, "")
	s.reloadCaddy()
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) toggleHost(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	id := chi.URLParam(r, "id")
	h, err := s.db.GetProxyHost(id)
	if err != nil || h == nil {
		errJSON(w, 404, "not found")
		return
	}
	h.Enabled = !h.Enabled
	h.UpdatedAt = time.Now()
	s.db.UpdateProxyHost(h)
	s.db.AuditLog(claims.UserID, "host_toggled", "host:"+h.ID, h.Domain)
	s.reloadCaddy()
	writeJSON(w, 200, h)
}

func (s *Server) toggleMaintenance(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	id := chi.URLParam(r, "id")
	h, err := s.db.GetProxyHost(id)
	if err != nil || h == nil {
		errJSON(w, 404, "not found")
		return
	}
	h.MaintenanceMode = !h.MaintenanceMode
	h.UpdatedAt = time.Now()
	s.db.UpdateProxyHost(h)
	action := "maintenance_off"
	if h.MaintenanceMode {
		action = "maintenance_on"
	}
	s.db.AuditLog(claims.UserID, action, "host:"+h.ID, h.Domain)
	s.reloadCaddy()
	writeJSON(w, 200, h)
}

// ── Access Rules ──────────────────────────────────────────────────────────────

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	rules, _ := s.db.ListAccessRules(chi.URLParam(r, "id"))
	if rules == nil {
		rules = []*db.AccessRule{}
	}
	writeJSON(w, 200, rules)
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	hostID := chi.URLParam(r, "id")
	var req struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := readJSON(r, &req); err != nil || req.Type == "" || req.Value == "" {
		errJSON(w, 400, "type and value required")
		return
	}

	value := req.Value
	if req.Type == "basicauth" {
		// Value comes in as "username:plaintext_password" — bcrypt the password
		parts := strings.SplitN(req.Value, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			errJSON(w, 400, "basicauth value must be username:password")
			return
		}
		hash, err := auth.HashPassword(parts[1])
		if err != nil {
			errJSON(w, 500, "hash password: "+err.Error())
			return
		}
		value = parts[0] + ":" + hash
	}

	rule := &db.AccessRule{ID: uuid.New().String(), HostID: hostID, Type: req.Type, Value: value}
	s.db.CreateAccessRule(rule)
	s.db.AuditLog(claims.UserID, "rule_created", "host:"+hostID, req.Type)
	s.reloadCaddy()
	writeJSON(w, 201, rule)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	hostID := chi.URLParam(r, "id")
	ruleID := chi.URLParam(r, "ruleID")
	s.db.DeleteAccessRule(ruleID)
	s.db.AuditLog(claims.UserID, "rule_deleted", "host:"+hostID, ruleID)
	s.reloadCaddy()
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ── Certificates ──────────────────────────────────────────────────────────────

func (s *Server) listCerts(w http.ResponseWriter, r *http.Request) {
	certs, _ := s.db.ListCertificates()
	if certs == nil {
		certs = []*db.Certificate{}
	}
	writeJSON(w, 200, certs)
}

func (s *Server) requestCert(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	hostID := chi.URLParam(r, "id")
	h, err := s.db.GetProxyHost(hostID)
	if err != nil || h == nil {
		errJSON(w, 404, "host not found")
		return
	}
	var req struct {
		Provider    string `json:"provider"`
		ACMEEmail   string `json:"acme_email"`
		DNSProvider string `json:"dns_provider"`
		DNSConfig   string `json:"dns_config"`
		CertPEM     string `json:"cert_pem"`
		KeyPEM      string `json:"key_pem"`
		ValidDays   int    `json:"valid_days"`
		Staging     bool   `json:"staging"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, "bad request")
		return
	}
	if req.Provider == "" {
		req.Provider = "letsencrypt"
	}

	cert := &db.Certificate{
		ID:          uuid.New().String(),
		HostID:      hostID,
		Provider:    req.Provider,
		SANs:        h.Domain,
		ACMEEmail:   req.ACMEEmail,
		DNSProvider: req.DNSProvider,
		DNSConfig:   req.DNSConfig,
		Staging:     req.Staging,
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	switch req.Provider {
	case "letsencrypt":
		if req.ACMEEmail == "" {
			errJSON(w, 400, "acme_email required for Let's Encrypt")
			return
		}

	case "selfsigned":
		days := req.ValidDays
		if days <= 0 {
			days = 365
		}
		expiry, certPEM, keyPEM, err := s.caddy.GenerateSelfSigned(h.Domain, days)
		if err != nil {
			errJSON(w, 500, "generate self-signed: "+err.Error())
			return
		}
		if err := s.caddy.StoreCert(s.dataDir, h.Domain, certPEM, keyPEM); err != nil {
			errJSON(w, 500, "store cert: "+err.Error())
			return
		}
		cert.Status = "active"
		cert.Expiry = expiry

	case "custom":
		if req.CertPEM == "" || req.KeyPEM == "" {
			errJSON(w, 400, "cert_pem and key_pem required")
			return
		}
		expiry, err := s.caddy.ValidateAndStoreCert(s.dataDir, h.Domain, req.CertPEM, req.KeyPEM)
		if err != nil {
			errJSON(w, 400, "invalid cert/key: "+err.Error())
			return
		}
		cert.Status = "active"
		cert.Expiry = expiry

	default:
		errJSON(w, 400, "unknown provider: "+req.Provider)
		return
	}

	s.db.UpsertCertificate(cert)
	s.db.AuditLog(claims.UserID, "cert_requested", "host:"+hostID, h.Domain+" ("+req.Provider+")")

	// Update host cert status
	h.CertStatus = cert.Status
	if !cert.Expiry.IsZero() {
		h.CertExpiry = cert.Expiry
		h.CertExpiryUnix = cert.Expiry.Unix()
	}
	h.UpdatedAt = time.Now()
	s.db.UpdateProxyHost(h)

	s.reloadCaddy()
	if s.checker != nil {
		s.checker.CheckNow(h)
	}
	writeJSON(w, 201, cert)
}

func (s *Server) getCert(w http.ResponseWriter, r *http.Request) {
	cert, _ := s.db.GetCertByHostID(chi.URLParam(r, "id"))
	if cert == nil {
		errJSON(w, 404, "no certificate")
		return
	}
	writeJSON(w, 200, cert)
}

func (s *Server) deleteCert(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	id := chi.URLParam(r, "id")

	cert, err := s.db.GetCertByID(id)
	if err != nil || cert == nil {
		errJSON(w, 404, "certificate not found")
		return
	}

	// Reset host cert status
	h, _ := s.db.GetProxyHost(cert.HostID)
	if h != nil {
		h.CertStatus = "none"
		h.CertExpiry = time.Time{}
		h.CertExpiryUnix = 0
		h.UpdatedAt = time.Now()
		s.db.UpdateProxyHost(h)
	}

	s.db.DeleteCertificate(id)
	s.db.AuditLog(claims.UserID, "cert_deleted", "cert:"+id, cert.SANs)
	s.reloadCaddy()
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ── Metric Destinations ───────────────────────────────────────────────────────

var validPrefix = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (s *Server) listDestinations(w http.ResponseWriter, r *http.Request) {
	dests, _ := s.db.ListMetricDestinations()
	if dests == nil {
		dests = []*db.MetricDestination{}
	}
	writeJSON(w, 200, dests)
}

func (s *Server) createDestination(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	var req db.MetricDestination
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, "bad request")
		return
	}
	if req.Name == "" || req.Type == "" || req.Host == "" || req.Port == 0 {
		errJSON(w, 400, "name, type, host, port required")
		return
	}
	if req.Prefix == "" {
		req.Prefix = "haberdasher"
	}
	if !validPrefix.MatchString(req.Prefix) {
		errJSON(w, 400, "prefix must match [a-zA-Z0-9._-]+")
		return
	}
	req.ID = uuid.New().String()
	req.CreatedAt = time.Now()
	req.Enabled = true
	s.db.CreateMetricDestination(&req)
	s.db.AuditLog(claims.UserID, "metric_dest_created", "dest:"+req.ID, req.Name)
	s.reloadMetrics()
	writeJSON(w, 201, req)
}

func (s *Server) updateDestination(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	id := chi.URLParam(r, "id")
	existing, _ := s.db.GetMetricDestination(id)
	if existing == nil {
		errJSON(w, 404, "not found")
		return
	}
	var req db.MetricDestination
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, "bad request")
		return
	}
	if req.Prefix == "" {
		req.Prefix = "haberdasher"
	}
	if !validPrefix.MatchString(req.Prefix) {
		errJSON(w, 400, "prefix must match [a-zA-Z0-9._-]+")
		return
	}
	req.ID = id
	s.db.UpdateMetricDestination(&req)
	s.db.AuditLog(claims.UserID, "metric_dest_updated", "dest:"+id, req.Name)
	s.reloadMetrics()
	writeJSON(w, 200, req)
}

func (s *Server) deleteDestination(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	id := chi.URLParam(r, "id")
	s.db.DeleteMetricDestination(id)
	s.db.AuditLog(claims.UserID, "metric_dest_deleted", "dest:"+id, "")
	s.reloadMetrics()
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) testDestination(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dest, _ := s.db.GetMetricDestination(id)
	if dest == nil {
		errJSON(w, 404, "not found")
		return
	}
	d := metrics.Destination{
		ID: dest.ID, Name: dest.Name, Type: dest.Type,
		Host: dest.Host, Port: dest.Port, Prefix: dest.Prefix,
		TLS: dest.TLS, ConfigJSON: dest.ConfigJSON, Enabled: true,
	}
	if err := metrics.SendTest(d); err != nil {
		errJSON(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) toggleDestination(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	id := chi.URLParam(r, "id")
	dest, _ := s.db.GetMetricDestination(id)
	if dest == nil {
		errJSON(w, 404, "not found")
		return
	}
	dest.Enabled = !dest.Enabled
	s.db.UpdateMetricDestination(dest)
	s.db.AuditLog(claims.UserID, "metric_dest_toggled", "dest:"+id, dest.Name)
	s.reloadMetrics()
	writeJSON(w, 200, dest)
}

// ── Settings ──────────────────────────────────────────────────────────────────

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	keys := []string{"admin_email", "instance_name"}
	out := map[string]string{}
	for _, k := range keys {
		v, _ := s.db.GetSetting(k)
		out[k] = v
	}
	writeJSON(w, 200, out)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFromRequest(r)
	var req map[string]string
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, "bad request")
		return
	}
	allowed := map[string]bool{"instance_name": true}
	for k, v := range req {
		if allowed[k] {
			s.db.SetSetting(k, v)
		}
	}
	s.db.AuditLog(claims.UserID, "settings_updated", "settings", "")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ── Audit Log ─────────────────────────────────────────────────────────────────

func (s *Server) auditLog(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.DB.Query(`SELECT id,user_id,action,target,detail,created_at FROM audit_log ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type entry struct {
		ID        string `json:"id"`
		UserID    string `json:"user_id"`
		Action    string `json:"action"`
		Target    string `json:"target"`
		Detail    string `json:"detail"`
		CreatedAt int64  `json:"created_at"`
	}
	var entries []entry
	for rows.Next() {
		var e entry
		rows.Scan(&e.ID, &e.UserID, &e.Action, &e.Target, &e.Detail, &e.CreatedAt)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []entry{}
	}
	writeJSON(w, 200, entries)
}
