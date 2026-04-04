package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/viche-ai/owl/internal/config"
	"github.com/viche-ai/owl/internal/engine"
	"github.com/viche-ai/owl/internal/ipc"
	"github.com/viche-ai/owl/internal/llm"
)

const SocketPath = "/tmp/owld.sock"

func main() {
	_ = os.Remove(SocketPath)

	cfg, err := config.Load()
	if err != nil {
		log.Println("Warning: Could not load config:", err)
		cfg = &config.Config{}
	}

	router := llm.NewRouter(cfg)

	log.Printf("owld starting with %d provider(s) configured", len(router.ListProviders()))
	for _, p := range router.ListProviders() {
		log.Printf("  → %s", p)
	}

	vicheURL, vicheToken := cfg.GetActiveRegistry()
	if vicheToken != "" {
		log.Printf("Viche registry: %s (private)", vicheURL)
	} else {
		log.Printf("Viche registry: %s (public)", vicheURL)
	}

	daemon := ipc.NewService()

	ipc.RunEngineHook = func(state *ipc.AgentState, mu func(func()), args *ipc.HatchArgs, inbox chan ipc.InboundMessage) {
		eng := &engine.AgentEngine{
			State:  state,
			Cfg:    cfg,
			Mu:     mu,
			Router: router,
		}
		eng.Run(args, inbox)
	}

	err = rpc.RegisterName("Daemon", daemon)
	if err != nil {
		log.Fatal("Format of service isn't correct: ", err)
	}

	listener, err := net.Listen("unix", SocketPath)
	if err != nil {
		log.Fatal("Listen error: ", err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("owld received shutdown signal, cleaning up agents...")
		daemon.Mu.Lock()
		for _, ch := range daemon.InboxChans {
			close(ch)
		}
		daemon.Mu.Unlock()

		// Wait a brief moment to allow Run functions to execute cleanup
		// (closing the channels and waiting for websocket disconnects)
		time.Sleep(1 * time.Second)
		_ = os.Remove(SocketPath)
		log.Println("owld shutdown complete.")
		os.Exit(0)
	}()

	log.Println("owld daemon listening on", SocketPath)

	go func() {
		http.HandleFunc("/api/v1/stream", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var event ipc.ExternalStreamEvent
			if err := json.Unmarshal(body, &event); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
			var reply ipc.StreamExternalReply
			if err := daemon.StreamExternalAgent(&event, &reply); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(reply); err != nil {
				log.Println("Failed to encode response:", err)
			}
		})
		log.Println("HTTP API listening on localhost:7890")
		if err := http.ListenAndServe("localhost:7890", nil); err != nil {
			log.Println("HTTP server error:", err)
		}
	}()

	rpc.Accept(listener)
}
