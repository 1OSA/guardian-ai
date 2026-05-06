package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"guardian-ai/internal/server"
)

// AppVersion is set at build time via -ldflags "-X main.AppVersion=x.y.z".
var AppVersion = "dev"

//go:embed frontend/dist**
var embeddedDist embed.FS

//go:embed ml-service/guardian_service.py ml-service/guardian_grpc.py ml-service/guardian_pb2.py ml-service/guardian_pb2_grpc.py ml-service/requirements.txt ml-service/guardian_model.onnx ml-service/tokenizer.json
var embeddedML embed.FS

func main() {
	listen := flag.String("listen", ":53", "DNS listen address (udp/tcp)")
	upstream := flag.String("upstream", "8.8.8.8:53 1.1.1.1:53", "Upstream DNS server(s), space-separated")
	blockfile := flag.String("blocklist", server.DefaultBlockfile, "Blocklist hosts-style file")
	mlAddr := flag.String("ml", "localhost:50051", "ML gRPC address")
	dbPath := flag.String("db", server.DefaultDBPath, "SQLite DB path")
	webAddr := flag.String("web", ":8081", "Web UI listen address")
	logLevelStr := flag.String("log-level", "warn", "Log level (error, warn, info, debug)")
	verbose := flag.Bool("verbose", false, "Enable info-level logging (shorthand for --log-level info)")
	frontendDev := flag.Bool("frontend-dev", false, "Enable CORS for Vite dev server at http://localhost:5173")
	flag.Parse()

	if *verbose && *logLevelStr == "warn" {
		*logLevelStr = "info"
	}

	if server.IsTermux() && *listen == ":53" {
		log.Printf("[guardian] WARNING: Termux detected — port 53 requires root.")
		log.Printf("[guardian] If bind fails, restart with: --listen :5353")
		log.Printf("[guardian] Then point your device DNS to <this-ip>:5353")
	}

	var logLevel server.LogLevel
	switch strings.ToLower(*logLevelStr) {
	case "error":
		logLevel = server.LogLevelError
	case "warn":
		logLevel = server.LogLevelWarn
	case "info":
		logLevel = server.LogLevelInfo
	case "debug":
		logLevel = server.LogLevelDebug
	default:
		logLevel = server.LogLevelInfo
	}

	// Extract and start the embedded ML service.
	td, err := os.MkdirTemp("", "guardian-ai-ml-*")
	if err != nil {
		log.Fatalf("failed to create temp dir for embedded ML: %v", err)
	}
	if err := server.WriteEmbeddedDir(embeddedML, "ml-service", td); err != nil {
		log.Fatalf("failed to extract embedded ML service: %v", err)
	}
	log.Printf("[ml] extracted embedded ML service to %s", td)

	// The model/tokenizer may already have been extracted by WriteEmbeddedDir
	// (if they were included in the go:embed directive). If not, try to copy
	// them from common disk locations next to the executable.
	exeDir := filepath.Dir(func() string { p, _ := os.Executable(); return p }())
	for _, modelFile := range []string{"tokenizer.json"} {
		dest := filepath.Join(td, modelFile)
		if _, err := os.Stat(dest); err == nil {
			log.Printf("[ml] %s found (embedded)", modelFile)
			continue
		}
		candidates := []string{
			filepath.Join(exeDir, modelFile),
			filepath.Join(exeDir, "ml-service", modelFile),
			filepath.Join("ml-service", modelFile),
			modelFile,
		}
		copied := false
		for _, src := range candidates {
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				log.Printf("[ml] warning: found %s at %s but could not copy: %v", modelFile, src, err)
			} else {
				log.Printf("[ml] copied %s from %s", modelFile, src)
				copied = true
			}
			break
		}
		if !copied {
			log.Printf("[ml] warning: %s not found — ML will fail until model is trained (run ml-service/train_model.py)", modelFile)
		}
	}

	mlCmd, err := server.StartEmbeddedPython(td)
	if err != nil {
		log.Fatalf("failed to start embedded python ML: %v", err)
	}

	// Wait for ML service to initialize (load model, tokenizer, start gRPC server)
	log.Printf("[ml] waiting for ML service to initialize...")
	time.Sleep(3 * time.Second)

	// srv is declared before the signal handler so the closure can reference it.
	var srv *server.GuardianServer

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Printf("[guardian] shutting down")
		if mlCmd != nil && mlCmd.Process != nil {
			_ = mlCmd.Process.Signal(syscall.SIGTERM)
		}
		if srv != nil {
			// DNS cache is now in-memory only; no persistence needed
		}
		os.Exit(0)
	}()

	srv, err = server.NewGuardianServer(*upstream, *mlAddr, *blockfile, *dbPath, logLevel, embeddedDist, AppVersion)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	if err := srv.StartServers(*listen, *webAddr, *frontendDev, true); err != nil {
		log.Fatalf("failed to start servers: %v", err)
	}

	select {} // block forever
}
