package main

import (
	"log"
	"net"
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
	rpc.Accept(listener)
}
