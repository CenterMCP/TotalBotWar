package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/center/TotalBotWar/internal/appversion"
	"github.com/center/TotalBotWar/internal/battle"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	address := flag.String("address", defaultAddress(), "Streamable HTTP listen address")
	endpoint := flag.String("endpoint", "/mcp", "Streamable HTTP endpoint path")
	stdio := flag.Bool("stdio", false, "Use stdio transport instead of HTTP")
	version := flag.Bool("version", false, "Print build version")
	flag.Parse()
	if *version {
		fmt.Println(appversion.String())
		return
	}
	if err := run(*address, *endpoint, *stdio); err != nil {
		log.Fatal(err)
	}
}
func defaultAddress() string {
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "3005"
	}
	return net.JoinHostPort(host, port)
}
func normalizeEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.ContainsAny(endpoint, "?# \t\r\n") {
		return "", errors.New("endpoint must be a non-empty URL path without query or fragment")
	}
	endpoint = "/" + strings.Trim(endpoint, "/")
	if endpoint == "/" || endpoint == "/health" {
		return "", errors.New("endpoint / and /health are reserved")
	}
	return endpoint, nil
}
func run(address, endpoint string, stdio bool) error {
	s := battle.NewServer(appversion.String(), os.Getenv("DATA_DIR"))
	if stdio {
		return server.ServeStdio(s)
	}
	path, err := normalizeEndpoint(endpoint)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	httpServer := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	transport := server.NewStreamableHTTPServer(s, server.WithStreamableHTTPServer(httpServer))
	mux.Handle(path, transport)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok","name":"TotalBotWar"}`)
	})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := transport.Shutdown(shutdown); err != nil {
			log.Printf("Shutdown: %v", err)
		}
	}()
	log.Printf("TotalBotWar %s listening at http://%s%s", appversion.String(), address, path)
	if err := transport.Start(address); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
