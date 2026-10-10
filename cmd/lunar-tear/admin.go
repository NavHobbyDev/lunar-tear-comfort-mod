package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"

	"lunar-tear/server/internal/runtime"
)

// startAdmin spins up the admin webhook used by external content tools to
// trigger an in-place re-read of assets/release/20240404193219.bin.e.
//
// Authentication: Bearer token via the LUNAR_ADMIN_TOKEN environment variable.
// If LUNAR_ADMIN_TOKEN is unset or empty the listener does not bind at all
// (fail closed), so a fresh deploy never exposes an unauthenticated endpoint.
//
// The default --admin-listen is 127.0.0.1:8082 so the webhook is only
// reachable via loopback unless the operator opts in by binding to 0.0.0.0.
func startAdmin(listen string, holder *runtime.Holder) {
	token := os.Getenv("LUNAR_ADMIN_TOKEN")
	if token == "" {
		log.Println("[admin] disabled (no LUNAR_ADMIN_TOKEN set)")
		return
	}
	expected := []byte("Bearer " + token)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/master-data/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		got := []byte(r.Header.Get("Authorization"))
		if len(got) != len(expected) || subtle.ConstantTimeCompare(got, expected) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := holder.Reload(); err != nil {
			log.Printf("[admin] master-data reload failed: %v", err)
			http.Error(w, "master-data reload failed", http.StatusInternalServerError)
			return
		}
		log.Printf("[admin] master-data reloaded by %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	log.Printf("[admin] webhook listener on %s (token-gated)", listen)
	go func() {
		if err := http.ListenAndServe(listen, mux); err != nil {
			log.Printf("[admin] webhook listener failed: %v", err)
		}
	}()
}
