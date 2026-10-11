package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"log"
	"net/http"

	"lunar-tear/server/internal/auth"

	_ "modernc.org/sqlite"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:3000", "HTTP listen address (host:port)")
	dbPath := flag.String("db", "db/auth.db", "SQLite database path for auth users")
	secret := flag.String("secret", "", "HMAC secret for tokens (auto-generated if empty)")
	noRegister := flag.Bool("no-register", false, "Disallow new account registrations for clients, when present. Default = false")
	flag.Parse()

	hmacSecret := []byte(*secret)
	if len(hmacSecret) == 0 {
		hmacSecret = make([]byte, 32)
		if _, err := rand.Read(hmacSecret); err != nil {
			log.Fatalf("generate secret: %v", err)
		}
		log.Printf("generated HMAC secret: %s", hex.EncodeToString(hmacSecret))
		log.Printf("pass --secret=%s to reuse across restarts", hex.EncodeToString(hmacSecret))
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	store, err := auth.NewAuthStore(db)
	if err != nil {
		log.Fatalf("init auth store: %v", err)
	}

	tok := auth.NewTokenService(hmacSecret)
	h := NewHandlers(store, tok, *noRegister)

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.HandleOAuth)
	mux.HandleFunc("/me", h.HandleMe)
	mux.HandleFunc("/check-username", h.HandleCheckUsername)

	log.Printf("auth server listening on %s", *listen)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
