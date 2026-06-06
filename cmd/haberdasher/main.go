package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/haberdasher/haberdasher/internal/api"
	caddymgr "github.com/haberdasher/haberdasher/internal/caddy"
	"github.com/haberdasher/haberdasher/internal/db"
	"github.com/haberdasher/haberdasher/internal/health"
	"github.com/haberdasher/haberdasher/internal/metrics"
	"github.com/haberdasher/haberdasher/web"
)

func main() {
	dataDir := envOr("HABERDASHER_DATA", "/data")
	listenAddr := envOr("HABERDASHER_LISTEN", ":8080")

	if err := os.MkdirAll(filepath.Join(dataDir, "caddy"), 0755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}

	database, err := db.Open(filepath.Join(dataDir, "haberdasher.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	jwtSecret, err := getOrCreateSecret(database)
	if err != nil {
		log.Fatalf("jwt secret: %v", err)
	}

	caddy := caddymgr.NewManager(dataDir)
	if err := caddy.Start(); err != nil {
		log.Printf("WARNING: caddy start failed: %v", err)
	} else {
		log.Println("Caddy started")
	}

	bus := metrics.NewBus()
	checker := health.NewChecker(database)
	checker.Start()

	srv := api.NewServer(database, caddy, bus, checker, jwtSecret, dataDir)
	router := srv.Router()

	staticFS, err := fs.Sub(web.StaticFiles, "dist")
	if err != nil {
		log.Printf("WARNING: web assets not embedded: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", router)

	if staticFS != nil {
		fileServer := http.FileServer(http.FS(staticFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			_, err := staticFS.Open(r.URL.Path[1:])
			if err != nil || r.URL.Path == "/" {
				f, err := staticFS.Open("index.html")
				if err != nil {
					http.Error(w, "index.html not found", 500)
					return
				}
				f.Close()
				http.ServeFileFS(w, r, staticFS, "index.html")
				return
			}
			fileServer.ServeHTTP(w, r)
		})
	}

	log.Printf("Haberdasher listening on %s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getOrCreateSecret(database *db.DB) (string, error) {
	secret, err := database.GetSetting("jwt_secret")
	if err != nil {
		return "", err
	}
	if secret != "" {
		return secret, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	secret = hex.EncodeToString(b)
	return secret, database.SetSetting("jwt_secret", secret)
}
