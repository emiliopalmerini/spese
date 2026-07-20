package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"spese/internal/demo"
	"spese/internal/kernel"
	"spese/internal/storage"
)

func main() {
	dbPath := flag.String("db", "tmp/spese-demo.db", "disposable SQLite demo database")
	flag.Parse()

	path := filepath.Clean(*dbPath)
	if !strings.Contains(strings.ToLower(filepath.Base(path)), "demo") {
		fatal(fmt.Errorf("refusing to replace %q: the database filename must contain demo", path))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal(fmt.Errorf("create demo directory: %w", err))
	}

	ctx := context.Background()
	store, err := storage.Open(ctx, path)
	if err != nil {
		fatal(err)
	}
	defer store.Close()

	if err := demo.Seed(ctx, store, kernel.Today()); err != nil {
		fatal(err)
	}
	fmt.Printf("demo data ready in %s\n", path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "spese-demo:", err)
	os.Exit(1)
}
