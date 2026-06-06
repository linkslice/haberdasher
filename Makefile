.PHONY: all build build-web build-go deps dev-web dev-go docker docker-run clean deploy-lxc

all: build

# Run this first on a fresh clone before anything else
deps:
	cd web && npm install
	go mod tidy

build: build-web build-go

build-web:
	cd web && npm install && npm run build

build-go: web/dist/index.html
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/haberdasher ./cmd/haberdasher

web/dist/index.html:
	$(MAKE) build-web

dev-web:
	cd web && npm run dev

dev-go:
	mkdir -p dev-data
	HABERDASHER_DATA=./dev-data go run ./cmd/haberdasher

# Run both dev servers (requires tmux or two terminals)
dev:
	@echo "Start two terminals:"
	@echo "  Terminal 1: make dev-web   (Vite on :5173, proxies /api to :8080)"
	@echo "  Terminal 2: make dev-go    (Go backend on :8080)"

docker:
	docker build -t haberdasher:latest .

docker-run:
	docker run -d \
		--name haberdasher \
		-p 80:80 \
		-p 443:443 \
		-p 8080:8080 \
		-v haberdasher-data:/data \
		--restart unless-stopped \
		haberdasher:latest

docker-logs:
	docker logs -f haberdasher

# Cross-compile for LXC (Linux amd64, no CGo)
deploy-lxc:
	cd web && npm install && npm run build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-s -w" -o bin/haberdasher-linux ./cmd/haberdasher
	@echo ""
	@echo "Binary ready: bin/haberdasher-linux"
	@echo ""
	@echo "Deploy to LXC:"
	@echo "  scp bin/haberdasher-linux root@<lxc-ip>:/usr/local/bin/haberdasher"
	@echo "  scp haberdasher.service root@<lxc-ip>:/etc/systemd/system/"
	@echo "  ssh root@<lxc-ip> 'systemctl daemon-reload && systemctl enable --now haberdasher'"

clean:
	rm -rf bin/ web/dist web/node_modules dev-data/
