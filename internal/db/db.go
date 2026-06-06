package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(path string) (*DB, error) {
	sqldb, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	d := &DB{sqldb}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

func (d *DB) migrate() error {
	_, err := d.Exec(`
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
	id            TEXT PRIMARY KEY,
	email         TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	totp_secret   TEXT,
	totp_enabled  INTEGER NOT NULL DEFAULT 0,
	role          TEXT NOT NULL DEFAULT 'admin',
	created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS proxy_hosts (
	id               TEXT PRIMARY KEY,
	domain           TEXT NOT NULL UNIQUE,
	upstream         TEXT NOT NULL,
	metrics_alias    TEXT,
	strip_path       INTEGER NOT NULL DEFAULT 0,
	websocket        INTEGER NOT NULL DEFAULT 1,
	force_https      INTEGER NOT NULL DEFAULT 1,
	enabled          INTEGER NOT NULL DEFAULT 1,
	health_status    TEXT NOT NULL DEFAULT 'unknown',
	health_latency_ms INTEGER NOT NULL DEFAULT 0,
	health_checked_at INTEGER NOT NULL DEFAULT 0,
	cert_status      TEXT NOT NULL DEFAULT 'none',
	cert_expiry      INTEGER NOT NULL DEFAULT 0,
	upstream_status  TEXT NOT NULL DEFAULT 'unknown',
	upstream_latency_ms INTEGER NOT NULL DEFAULT 0,
	upstream_checked_at INTEGER NOT NULL DEFAULT 0,
	maintenance_mode INTEGER NOT NULL DEFAULT 0,
	created_at       INTEGER NOT NULL,
	updated_at       INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS certificates (
	id          TEXT PRIMARY KEY,
	host_id     TEXT NOT NULL REFERENCES proxy_hosts(id) ON DELETE CASCADE,
	provider    TEXT NOT NULL DEFAULT 'letsencrypt',
	sans        TEXT NOT NULL,
	acme_email  TEXT NOT NULL,
	dns_provider TEXT,
	dns_config  TEXT,
	expiry      INTEGER,
	status      TEXT NOT NULL DEFAULT 'pending',
	staging     INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS access_rules (
	id       TEXT PRIMARY KEY,
	host_id  TEXT NOT NULL REFERENCES proxy_hosts(id) ON DELETE CASCADE,
	type     TEXT NOT NULL,
	value    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS metric_destinations (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	type        TEXT NOT NULL,
	host        TEXT NOT NULL,
	port        INTEGER NOT NULL,
	prefix      TEXT NOT NULL DEFAULT 'haberdasher',
	tls         INTEGER NOT NULL DEFAULT 0,
	config_json TEXT,
	enabled     INTEGER NOT NULL DEFAULT 1,
	created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
	id         TEXT PRIMARY KEY,
	user_id    TEXT,
	action     TEXT NOT NULL,
	target     TEXT,
	detail     TEXT,
	created_at INTEGER NOT NULL
);
`)
	if err != nil {
		return err
	}

	// Migrations for existing databases — ADD COLUMN is idempotent via IF NOT EXISTS workaround
	migrations := []string{
		`ALTER TABLE proxy_hosts ADD COLUMN health_status TEXT NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE proxy_hosts ADD COLUMN health_latency_ms INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE proxy_hosts ADD COLUMN health_checked_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE proxy_hosts ADD COLUMN cert_status TEXT NOT NULL DEFAULT 'none'`,
		`ALTER TABLE proxy_hosts ADD COLUMN cert_expiry INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE proxy_hosts ADD COLUMN upstream_status TEXT NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE proxy_hosts ADD COLUMN upstream_latency_ms INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE proxy_hosts ADD COLUMN upstream_checked_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE proxy_hosts ADD COLUMN maintenance_mode INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE certificates ADD COLUMN staging INTEGER NOT NULL DEFAULT 0`,
	}
	for _, m := range migrations {
		d.Exec(m) // ignore error — column already exists
	}

	// Fix hosts that got cert_status='pending' by default with no actual cert request
	d.Exec(`UPDATE proxy_hosts SET cert_status='none'
		WHERE cert_status='pending'
		AND id NOT IN (SELECT host_id FROM certificates)`)

	return nil
}

// Setting helpers

func (d *DB) GetSetting(key string) (string, error) {
	var val string
	err := d.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (d *DB) IsSetupComplete() (bool, error) {
	v, err := d.GetSetting("setup_complete")
	return v == "true", err
}

// User models

type User struct {
	ID           string
	Email        string
	PasswordHash string
	TOTPSecret   string
	TOTPEnabled  bool
	Role         string
	CreatedAt    time.Time
}

func (d *DB) CreateUser(u *User) error {
	_, err := d.Exec(`INSERT INTO users(id,email,password_hash,totp_secret,totp_enabled,role,created_at) VALUES(?,?,?,?,?,?,?)`,
		u.ID, u.Email, u.PasswordHash, u.TOTPSecret, boolInt(u.TOTPEnabled), u.Role, u.CreatedAt.Unix())
	return err
}

func (d *DB) GetUserByEmail(email string) (*User, error) {
	u := &User{}
	var ts sql.NullString
	var tot int
	var ca int64
	err := d.QueryRow(`SELECT id,email,password_hash,totp_secret,totp_enabled,role,created_at FROM users WHERE email=?`, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &ts, &tot, &u.Role, &ca)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.TOTPSecret = ts.String
	u.TOTPEnabled = tot == 1
	u.CreatedAt = time.Unix(ca, 0)
	return u, nil
}

func (d *DB) GetUserByID(id string) (*User, error) {
	u := &User{}
	var ts sql.NullString
	var tot int
	var ca int64
	err := d.QueryRow(`SELECT id,email,password_hash,totp_secret,totp_enabled,role,created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &ts, &tot, &u.Role, &ca)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.TOTPSecret = ts.String
	u.TOTPEnabled = tot == 1
	u.CreatedAt = time.Unix(ca, 0)
	return u, nil
}

func (d *DB) UpdateUserTOTP(id, secret string, enabled bool) error {
	_, err := d.Exec(`UPDATE users SET totp_secret=?, totp_enabled=? WHERE id=?`, secret, boolInt(enabled), id)
	return err
}

func (d *DB) UpdateUserPassword(id, hash string) error {
	_, err := d.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	return err
}

// ProxyHost models

type ProxyHost struct {
	ID                 string    `json:"ID"`
	Domain             string    `json:"Domain"`
	Upstream           string    `json:"Upstream"`
	MetricsAlias       string    `json:"MetricsAlias"`
	StripPath          bool      `json:"StripPath"`
	WebSocket          bool      `json:"WebSocket"`
	ForceHTTPS         bool      `json:"ForceHTTPS"`
	Enabled            bool      `json:"Enabled"`
	MaintenanceMode    bool      `json:"MaintenanceMode"`
	HealthStatus       string    `json:"HealthStatus"`
	HealthLatencyMs    int       `json:"HealthLatencyMs"`
	HealthCheckedAt    time.Time `json:"-"`
	CertStatus         string    `json:"CertStatus"`
	CertExpiry         time.Time `json:"-"`
	CertExpiryUnix     int64     `json:"CertExpiry"`
	UpstreamStatus     string    `json:"UpstreamStatus"`
	UpstreamLatencyMs  int       `json:"UpstreamLatencyMs"`
	UpstreamCheckedAt  time.Time `json:"-"`
	CreatedAt          time.Time `json:"-"`
	CreatedAtUnix      int64     `json:"CreatedAt"`
	UpdatedAt          time.Time `json:"-"`
	UpdatedAtUnix      int64     `json:"UpdatedAt"`
}

func (d *DB) CreateProxyHost(h *ProxyHost) error {
	_, err := d.Exec(`INSERT INTO proxy_hosts
		(id,domain,upstream,metrics_alias,strip_path,websocket,force_https,enabled,
		 health_status,health_latency_ms,health_checked_at,cert_status,cert_expiry,
		 upstream_status,upstream_latency_ms,upstream_checked_at,maintenance_mode,
		 created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.Domain, h.Upstream, h.MetricsAlias,
		boolInt(h.StripPath), boolInt(h.WebSocket), boolInt(h.ForceHTTPS), boolInt(h.Enabled),
		h.HealthStatus, h.HealthLatencyMs, h.HealthCheckedAt.Unix(),
		h.CertStatus, h.CertExpiry.Unix(),
		h.UpstreamStatus, h.UpstreamLatencyMs, h.UpstreamCheckedAt.Unix(),
		boolInt(h.MaintenanceMode),
		h.CreatedAt.Unix(), h.UpdatedAt.Unix())
	return err
}

func (d *DB) UpdateProxyHost(h *ProxyHost) error {
	_, err := d.Exec(`UPDATE proxy_hosts SET
		domain=?,upstream=?,metrics_alias=?,strip_path=?,websocket=?,force_https=?,enabled=?,
		health_status=?,health_latency_ms=?,health_checked_at=?,cert_status=?,cert_expiry=?,
		upstream_status=?,upstream_latency_ms=?,upstream_checked_at=?,maintenance_mode=?,
		updated_at=?
		WHERE id=?`,
		h.Domain, h.Upstream, h.MetricsAlias,
		boolInt(h.StripPath), boolInt(h.WebSocket), boolInt(h.ForceHTTPS), boolInt(h.Enabled),
		h.HealthStatus, h.HealthLatencyMs, h.HealthCheckedAt.Unix(),
		h.CertStatus, h.CertExpiry.Unix(),
		h.UpstreamStatus, h.UpstreamLatencyMs, h.UpstreamCheckedAt.Unix(),
		boolInt(h.MaintenanceMode),
		h.UpdatedAt.Unix(), h.ID)
	return err
}

func (d *DB) DeleteProxyHost(id string) error {
	_, err := d.Exec(`DELETE FROM proxy_hosts WHERE id=?`, id)
	return err
}

func (d *DB) GetProxyHost(id string) (*ProxyHost, error) {
	row := d.QueryRow(`SELECT id,domain,upstream,metrics_alias,strip_path,websocket,force_https,enabled,
		health_status,health_latency_ms,health_checked_at,cert_status,cert_expiry,
		upstream_status,upstream_latency_ms,upstream_checked_at,maintenance_mode,
		created_at,updated_at FROM proxy_hosts WHERE id=?`, id)
	return scanHost(row)
}

func (d *DB) ListProxyHosts() ([]*ProxyHost, error) {
	rows, err := d.Query(`SELECT id,domain,upstream,metrics_alias,strip_path,websocket,force_https,enabled,
		health_status,health_latency_ms,health_checked_at,cert_status,cert_expiry,
		upstream_status,upstream_latency_ms,upstream_checked_at,maintenance_mode,
		created_at,updated_at FROM proxy_hosts ORDER BY domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []*ProxyHost
	for rows.Next() {
		h, err := scanHostRow(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

func scanHost(row *sql.Row) (*ProxyHost, error) {
	h := &ProxyHost{}
	var ma sql.NullString
	var sp, ws, fh, en, mm int
	var hca, ca, ua, certExp, uca int64
	err := row.Scan(
		&h.ID, &h.Domain, &h.Upstream, &ma,
		&sp, &ws, &fh, &en,
		&h.HealthStatus, &h.HealthLatencyMs, &hca,
		&h.CertStatus, &certExp,
		&h.UpstreamStatus, &h.UpstreamLatencyMs, &uca,
		&mm, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h.MetricsAlias = ma.String
	h.StripPath = sp == 1
	h.WebSocket = ws == 1
	h.ForceHTTPS = fh == 1
	h.Enabled = en == 1
	h.MaintenanceMode = mm == 1
	h.HealthCheckedAt = time.Unix(hca, 0)
	h.CertExpiry = time.Unix(certExp, 0)
	h.CertExpiryUnix = certExp
	h.UpstreamCheckedAt = time.Unix(uca, 0)
	h.CreatedAt = time.Unix(ca, 0)
	h.CreatedAtUnix = ca
	h.UpdatedAt = time.Unix(ua, 0)
	h.UpdatedAtUnix = ua
	if h.HealthStatus == "" { h.HealthStatus = "unknown" }
	if h.CertStatus == "" { h.CertStatus = "none" }
	if h.UpstreamStatus == "" { h.UpstreamStatus = "unknown" }
	return h, nil
}

func scanHostRow(rows *sql.Rows) (*ProxyHost, error) {
	h := &ProxyHost{}
	var ma sql.NullString
	var sp, ws, fh, en, mm int
	var hca, ca, ua, certExp, uca int64
	err := rows.Scan(
		&h.ID, &h.Domain, &h.Upstream, &ma,
		&sp, &ws, &fh, &en,
		&h.HealthStatus, &h.HealthLatencyMs, &hca,
		&h.CertStatus, &certExp,
		&h.UpstreamStatus, &h.UpstreamLatencyMs, &uca,
		&mm, &ca, &ua)
	if err != nil {
		return nil, err
	}
	h.MetricsAlias = ma.String
	h.StripPath = sp == 1
	h.WebSocket = ws == 1
	h.ForceHTTPS = fh == 1
	h.Enabled = en == 1
	h.MaintenanceMode = mm == 1
	h.HealthCheckedAt = time.Unix(hca, 0)
	h.CertExpiry = time.Unix(certExp, 0)
	h.CertExpiryUnix = certExp
	h.UpstreamCheckedAt = time.Unix(uca, 0)
	h.CreatedAt = time.Unix(ca, 0)
	h.CreatedAtUnix = ca
	h.UpdatedAt = time.Unix(ua, 0)
	h.UpdatedAtUnix = ua
	if h.HealthStatus == "" { h.HealthStatus = "unknown" }
	if h.CertStatus == "" { h.CertStatus = "none" }
	if h.UpstreamStatus == "" { h.UpstreamStatus = "unknown" }
	return h, nil
}

// Certificate models

type Certificate struct {
	ID          string
	HostID      string
	Provider    string
	SANs        string
	ACMEEmail   string
	DNSProvider string
	DNSConfig   string
	Expiry      time.Time
	Status      string
	Staging     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (d *DB) UpsertCertificate(c *Certificate) error {
	_, err := d.Exec(`INSERT INTO certificates(id,host_id,provider,sans,acme_email,dns_provider,dns_config,expiry,status,staging,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status,expiry=excluded.expiry,staging=excluded.staging,updated_at=excluded.updated_at`,
		c.ID, c.HostID, c.Provider, c.SANs, c.ACMEEmail, c.DNSProvider, c.DNSConfig, c.Expiry.Unix(), c.Status, boolInt(c.Staging), c.CreatedAt.Unix(), c.UpdatedAt.Unix())
	return err
}

func (d *DB) GetCertByID(id string) (*Certificate, error) {
	c := &Certificate{}
	var dp, dc sql.NullString
	var exp, ca, ua int64
	var staging int
	err := d.QueryRow(`SELECT id,host_id,provider,sans,acme_email,dns_provider,dns_config,expiry,status,staging,created_at,updated_at FROM certificates WHERE id=?`, id).
		Scan(&c.ID, &c.HostID, &c.Provider, &c.SANs, &c.ACMEEmail, &dp, &dc, &exp, &c.Status, &staging, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.DNSProvider = dp.String
	c.DNSConfig = dc.String
	c.Expiry = time.Unix(exp, 0)
	c.Staging = staging == 1
	c.CreatedAt = time.Unix(ca, 0)
	c.UpdatedAt = time.Unix(ua, 0)
	return c, nil
}

func (d *DB) DeleteCertificate(id string) error {
	_, err := d.Exec(`DELETE FROM certificates WHERE id=?`, id)
	return err
}

func (d *DB) GetCertByHostID(hostID string) (*Certificate, error) {
	c := &Certificate{}
	var dp, dc sql.NullString
	var exp, ca, ua int64
	var staging2 int
	err := d.QueryRow(`SELECT id,host_id,provider,sans,acme_email,dns_provider,dns_config,expiry,status,staging,created_at,updated_at FROM certificates WHERE host_id=?`, hostID).
		Scan(&c.ID, &c.HostID, &c.Provider, &c.SANs, &c.ACMEEmail, &dp, &dc, &exp, &c.Status, &staging2, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.DNSProvider = dp.String
	c.DNSConfig = dc.String
	c.Expiry = time.Unix(exp, 0)
	c.Staging = staging2 == 1
	c.CreatedAt = time.Unix(ca, 0)
	c.UpdatedAt = time.Unix(ua, 0)
	return c, nil
}

func (d *DB) ListCertificates() ([]*Certificate, error) {
	rows, err := d.Query(`SELECT id,host_id,provider,sans,acme_email,dns_provider,dns_config,expiry,status,staging,created_at,updated_at FROM certificates ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var certs []*Certificate
	for rows.Next() {
		c := &Certificate{}
		var dp, dc sql.NullString
		var exp, ca, ua int64
		var s int
		if err := rows.Scan(&c.ID, &c.HostID, &c.Provider, &c.SANs, &c.ACMEEmail, &dp, &dc, &exp, &c.Status, &s, &ca, &ua); err != nil {
			return nil, err
		}
		c.DNSProvider = dp.String
		c.DNSConfig = dc.String
		c.Expiry = time.Unix(exp, 0)
		c.Staging = s == 1
		c.CreatedAt = time.Unix(ca, 0)
		c.UpdatedAt = time.Unix(ua, 0)
		certs = append(certs, c)
	}
	return certs, nil
}

// AccessRule models

type AccessRule struct {
	ID     string
	HostID string
	Type   string
	Value  string
}

func (d *DB) CreateAccessRule(r *AccessRule) error {
	_, err := d.Exec(`INSERT INTO access_rules(id,host_id,type,value) VALUES(?,?,?,?)`, r.ID, r.HostID, r.Type, r.Value)
	return err
}

func (d *DB) DeleteAccessRule(id string) error {
	_, err := d.Exec(`DELETE FROM access_rules WHERE id=?`, id)
	return err
}

func (d *DB) ListAccessRules(hostID string) ([]*AccessRule, error) {
	rows, err := d.Query(`SELECT id,host_id,type,value FROM access_rules WHERE host_id=?`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []*AccessRule
	for rows.Next() {
		r := &AccessRule{}
		if err := rows.Scan(&r.ID, &r.HostID, &r.Type, &r.Value); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// MetricDestination models

type MetricDestination struct {
	ID         string
	Name       string
	Type       string
	Host       string
	Port       int
	Prefix     string
	TLS        bool
	ConfigJSON string
	Enabled    bool
	CreatedAt  time.Time
}

func (d *DB) CreateMetricDestination(m *MetricDestination) error {
	_, err := d.Exec(`INSERT INTO metric_destinations(id,name,type,host,port,prefix,tls,config_json,enabled,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.Name, m.Type, m.Host, m.Port, m.Prefix, boolInt(m.TLS), m.ConfigJSON, boolInt(m.Enabled), m.CreatedAt.Unix())
	return err
}

func (d *DB) UpdateMetricDestination(m *MetricDestination) error {
	_, err := d.Exec(`UPDATE metric_destinations SET name=?,type=?,host=?,port=?,prefix=?,tls=?,config_json=?,enabled=? WHERE id=?`,
		m.Name, m.Type, m.Host, m.Port, m.Prefix, boolInt(m.TLS), m.ConfigJSON, boolInt(m.Enabled), m.ID)
	return err
}

func (d *DB) DeleteMetricDestination(id string) error {
	_, err := d.Exec(`DELETE FROM metric_destinations WHERE id=?`, id)
	return err
}

func (d *DB) ListMetricDestinations() ([]*MetricDestination, error) {
	rows, err := d.Query(`SELECT id,name,type,host,port,prefix,tls,config_json,enabled,created_at FROM metric_destinations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dests []*MetricDestination
	for rows.Next() {
		m := &MetricDestination{}
		var cj sql.NullString
		var tls, en int
		var ca int64
		if err := rows.Scan(&m.ID, &m.Name, &m.Type, &m.Host, &m.Port, &m.Prefix, &tls, &cj, &en, &ca); err != nil {
			return nil, err
		}
		m.TLS = tls == 1
		m.ConfigJSON = cj.String
		m.Enabled = en == 1
		m.CreatedAt = time.Unix(ca, 0)
		dests = append(dests, m)
	}
	return dests, nil
}

func (d *DB) GetMetricDestination(id string) (*MetricDestination, error) {
	m := &MetricDestination{}
	var cj sql.NullString
	var tls, en int
	var ca int64
	err := d.QueryRow(`SELECT id,name,type,host,port,prefix,tls,config_json,enabled,created_at FROM metric_destinations WHERE id=?`, id).
		Scan(&m.ID, &m.Name, &m.Type, &m.Host, &m.Port, &m.Prefix, &tls, &cj, &en, &ca)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.TLS = tls == 1
	m.ConfigJSON = cj.String
	m.Enabled = en == 1
	m.CreatedAt = time.Unix(ca, 0)
	return m, nil
}

// AuditLog

func (d *DB) AuditLog(userID, action, target, detail string) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	d.Exec(`INSERT INTO audit_log(id,user_id,action,target,detail,created_at) VALUES(?,?,?,?,?,?)`,
		id, userID, action, target, detail, time.Now().Unix())
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
