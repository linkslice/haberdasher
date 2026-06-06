package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Event is emitted for every proxied request
type Event struct {
	Host       string
	HostAlias  string
	Method     string
	StatusCode int
	BytesIn    int64
	BytesOut   int64
	DurationMs int64
	Timestamp  time.Time
}

// Destination mirrors db.MetricDestination without import cycle
type Destination struct {
	ID         string
	Name       string
	Type       string
	Host       string
	Port       int
	Prefix     string
	TLS        bool
	ConfigJSON string
	Enabled    bool
}

type graphiteConfig struct {
	Protocol        string `json:"protocol"`
	FlushIntervalS  int    `json:"flush_interval_s"`
}

type influxConfig struct {
	Version        int    `json:"version"`
	Org            string `json:"org"`
	Bucket         string `json:"bucket"`
	Token          string `json:"token"`
	DB             string `json:"db"`
	FlushIntervalS int    `json:"flush_interval_s"`
}

// Bus receives events and fans out to all enabled destinations
type Bus struct {
	ch    chan Event
	mu    sync.RWMutex
	dests []Destination
}

func NewBus() *Bus {
	b := &Bus{
		ch: make(chan Event, 2000),
	}
	go b.dispatch()
	return b
}

func (b *Bus) Emit(e Event) {
	select {
	case b.ch <- e:
	default:
		// drop on full buffer — never block hot path
	}
}

func (b *Bus) SetDestinations(dests []Destination) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dests = dests
}

func (b *Bus) dispatch() {
	for e := range b.ch {
		b.mu.RLock()
		dests := make([]Destination, len(b.dests))
		copy(dests, b.dests)
		b.mu.RUnlock()
		for _, d := range dests {
			if !d.Enabled {
				continue
			}
			switch d.Type {
			case "statsd":
				go emitStatsd(d, e)
			case "graphite":
				go emitGraphite(d, e)
			case "influxdb":
				go emitInflux(d, e)
			}
		}
	}
}

// SendTest fires a synthetic event to a single destination for the "test" button
func SendTest(d Destination) error {
	e := Event{
		Host:       "haberdasher.test",
		HostAlias:  "test",
		Method:     "GET",
		StatusCode: 200,
		BytesIn:    128,
		BytesOut:   512,
		DurationMs: 5,
		Timestamp:  time.Now(),
	}
	switch d.Type {
	case "statsd":
		return emitStatsd(d, e)
	case "graphite":
		return emitGraphite(d, e)
	case "influxdb":
		return emitInflux(d, e)
	}
	return fmt.Errorf("unknown destination type: %s", d.Type)
}

// hostSlug returns the metrics alias if set, otherwise sanitizes the domain
func hostSlug(e Event) string {
	if e.HostAlias != "" {
		return sanitize(e.HostAlias)
	}
	return sanitize(e.Host)
}

var nonAlphanumDot = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitize(s string) string {
	return nonAlphanumDot.ReplaceAllString(s, "_")
}

func statusClass(code int) string {
	switch {
	case code < 200:
		return "1xx"
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

// ── StatsD ──────────────────────────────────────────────────────────────────

func emitStatsd(d Destination, e Event) error {
	addr := fmt.Sprintf("%s:%d", d.Host, d.Port)
	conn, err := net.DialTimeout("udp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	slug := hostSlug(e)
	pfx := strings.TrimRight(d.Prefix, ".")
	base := fmt.Sprintf("%s.proxy.%s", pfx, slug)

	msgs := []string{
		fmt.Sprintf("%s.requests:1|c", base),
		fmt.Sprintf("%s.bytes_in:%d|c", base, e.BytesIn),
		fmt.Sprintf("%s.bytes_out:%d|c", base, e.BytesOut),
		fmt.Sprintf("%s.response_ms:%d|ms", base, e.DurationMs),
		fmt.Sprintf("%s.status.%s:1|c", base, statusClass(e.StatusCode)),
	}

	payload := strings.Join(msgs, "\n")
	_, err = conn.Write([]byte(payload))
	return err
}

// ── Graphite ─────────────────────────────────────────────────────────────────

func emitGraphite(d Destination, e Event) error {
	var cfg graphiteConfig
	if d.ConfigJSON != "" {
		json.Unmarshal([]byte(d.ConfigJSON), &cfg)
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "tcp"
	}

	addr := fmt.Sprintf("%s:%d", d.Host, d.Port)
	conn, err := net.DialTimeout(cfg.Protocol, addr, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	slug := hostSlug(e)
	pfx := strings.TrimRight(d.Prefix, ".")
	base := fmt.Sprintf("%s.proxy.%s", pfx, slug)
	ts := e.Timestamp.Unix()

	lines := []string{
		fmt.Sprintf("%s.requests 1 %d", base, ts),
		fmt.Sprintf("%s.bytes_in %d %d", base, e.BytesIn, ts),
		fmt.Sprintf("%s.bytes_out %d %d", base, e.BytesOut, ts),
		fmt.Sprintf("%s.response_ms %d %d", base, e.DurationMs, ts),
		fmt.Sprintf("%s.status.%s 1 %d", base, statusClass(e.StatusCode), ts),
	}

	_, err = fmt.Fprint(conn, strings.Join(lines, "\n")+"\n")
	return err
}

// ── InfluxDB ──────────────────────────────────────────────────────────────────

func emitInflux(d Destination, e Event) error {
	var cfg influxConfig
	if d.ConfigJSON != "" {
		json.Unmarshal([]byte(d.ConfigJSON), &cfg)
	}
	if cfg.Version == 0 {
		cfg.Version = 2
	}

	slug := hostSlug(e)
	pfx := strings.TrimRight(d.Prefix, "_")
	measurement := pfx + "_proxy"

	// Line protocol: measurement,tags fields timestamp
	tags := fmt.Sprintf("host=%s,host_slug=%s,status_class=%s,method=%s",
		escapeInflux(e.Host),
		escapeInflux(slug),
		statusClass(e.StatusCode),
		e.Method,
	)
	if e.HostAlias != "" {
		tags += ",alias=" + escapeInflux(e.HostAlias)
	}

	fields := fmt.Sprintf("requests=1i,bytes_in=%di,bytes_out=%di,response_ms=%di,status_code=%di",
		e.BytesIn, e.BytesOut, e.DurationMs, e.StatusCode)

	line := fmt.Sprintf("%s,%s %s %d\n", measurement, tags, fields, e.Timestamp.UnixNano())

	scheme := "http"
	if d.TLS {
		scheme = "https"
	}

	var url string
	var req *http.Request
	var err error

	if cfg.Version == 2 {
		url = fmt.Sprintf("%s://%s:%d/api/v2/write?org=%s&bucket=%s&precision=ns",
			scheme, d.Host, d.Port, cfg.Org, cfg.Bucket)
		req, err = http.NewRequest("POST", url, bytes.NewBufferString(line))
		if err != nil {
			return err
		}
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Token "+cfg.Token)
		}
	} else {
		db := cfg.DB
		if db == "" {
			db = "haberdasher"
		}
		url = fmt.Sprintf("%s://%s:%d/write?db=%s&precision=ns", scheme, d.Host, d.Port, db)
		req, err = http.NewRequest("POST", url, bytes.NewBufferString(line))
		if err != nil {
			return err
		}
	}

	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("influxdb write failed %d: %s", resp.StatusCode, body)
	}
	return nil
}

func escapeInflux(s string) string {
	s = strings.ReplaceAll(s, " ", "\\ ")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "=", "\\=")
	return s
}
