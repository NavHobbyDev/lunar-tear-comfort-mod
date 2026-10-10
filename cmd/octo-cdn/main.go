package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/service"
	"lunar-tear/server/internal/store/sqlite"
	"lunar-tear/server/migrations"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:8080", "local bind address (host:port)")
	publicAddr := flag.String("public-addr", "127.0.0.1:8080", "externally-reachable host:port used for list.bin URL rewriting")
	assetsDir := flag.String("assets-dir", ".", "root directory containing the assets/ tree")
	dbPath := flag.String("db", "db/game.db", "SQLite database path (shared with lunar-tear; serves the /information options page)")
	flag.Parse()

	// Build resourcesBaseURL from public-addr (must be exactly 43 chars to fit in list.bin protobuf).
	prefix := "http://" + *publicAddr + "/"
	padLen := 43 - len(prefix)
	resourcesBaseURL := ""
	if padLen < 1 {
		log.Printf("[config] public-addr too long for 43-char resource URL; list.bin will be served unchanged")
	} else {
		resourcesBaseURL = prefix + strings.Repeat("r", padLen)
	}

	// Open the game database so the /information options page can load/save
	// per-user settings (WAL mode supports the concurrent lunar-tear writer).
	db, err := database.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := migrations.Up(context.Background(), db); err != nil {
		log.Fatalf("run migrations: %v", err)
	}
	log.Printf("database opened: %s", *dbPath)

	userStore := sqlite.New(db, nil)

	octoServer := service.NewOctoHTTPServer(resourcesBaseURL, *assetsDir, userStore)
	h2s := &http2.Server{}
	handler := h2c.NewHandler(octoServer.Handler(), h2s)

	srv := &http.Server{
		Addr:    *listen,
		Handler: handler,
	}
	http2.ConfigureServer(srv, h2s)

	// Resolve actual listen address for logging (useful when port is 0).
	lis, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *listen, err)
	}
	log.Printf("Octo CDN listening on %s (HTTP/1.1 + h2c)", lis.Addr())
	log.Printf("public address: %s", *publicAddr)
	if *assetsDir != "." {
		log.Printf("assets directory: %s", *assetsDir)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Serve(lis); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	srv.Shutdown(context.Background())
	log.Println("shutdown complete")
}
