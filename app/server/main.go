package main

import (
	"fmt"
	"log"
	"os"
	"plandex-server/model"
	"plandex-server/routes"
	"plandex-server/setup"

	"github.com/gorilla/mux"
)

func main() {
	// Configure the default logger to include milliseconds in timestamps
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	routes.RegisterHandlePlandex(func(router *mux.Router, path string, isStreaming bool, handler routes.PlandexHandler) *mux.Route {
		return router.HandleFunc(path, handler)
	})

	err := model.EnsureLiteLLM(2)
	if err != nil {
		panic(fmt.Sprintf("Failed to start LiteLLM proxy: %v", err))
	}
	setup.RegisterShutdownHook(func() {
		model.ShutdownLiteLLMServer()
	})

	// Initialize error handling infrastructure
	model.InitGlobalCircuitBreaker()
	log.Println("Initialized global circuit breaker")

	model.InitGlobalStreamRecoveryManager()
	log.Println("Initialized global stream recovery manager")

	model.InitGlobalHealthCheckManager()
	log.Println("Initialized global health check manager")

	model.InitGlobalDegradationManager()
	log.Println("Initialized global degradation manager")

	model.InitGlobalDeadLetterQueue()
	log.Println("Initialized global dead letter queue")

	// Register cleanup for error handling components
	setup.RegisterShutdownHook(func() {
		if model.GlobalCircuitBreaker != nil {
			log.Println("Circuit breaker final metrics:", model.GlobalCircuitBreaker.GetMetrics())
		}
		if model.GlobalStreamRecoveryManager != nil {
			log.Println("Stream recovery final stats:", model.GlobalStreamRecoveryManager.GetStats())
		}
		if model.GlobalHealthCheckManager != nil {
			log.Println("Health check final metrics:", model.GlobalHealthCheckManager.GetMetrics())
		}
		if model.GlobalDegradationManager != nil {
			log.Println("Degradation final metrics:", model.GlobalDegradationManager.GetMetrics())
		}
		if model.GlobalDeadLetterQueue != nil {
			log.Println("Dead letter queue final stats:", model.GlobalDeadLetterQueue.GetStats())
		}
	})

	r := mux.NewRouter()
	routes.AddHealthRoutes(r)
	routes.AddApiRoutes(r)
	routes.AddProxyableApiRoutes(r)
	setup.MustLoadIp()
	setup.MustInitDb()
	setup.StartServer(r, nil, nil)
	os.Exit(0)
}
