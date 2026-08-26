// Command server is the runnable entry point for the rock wool facade render
// handover service. It opens the embedded store, runs recovery verification,
// wires the transactional engine into the HTTP API and starts the listener.
package main

import (
	"log"
	"net/http"
	"os"

	"rockwool-facade-render-handover/internal/httpapi"
	"rockwool-facade-render-handover/internal/store"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "rockwool.db"
	}

	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	engine := store.NewEngine(db)
	ver, err := engine.Verify()
	if err != nil {
		log.Fatalf("recovery verification: %v", err)
	}
	if !ver.OK {
		log.Printf("WARNING: recovery found violations, entering read-only isolation: %v", ver.Violations)
	}

	srv := httpapi.NewServer(engine)
	log.Printf("rock wool facade handover service listening on %s (db=%s)", addr, dbPath)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
