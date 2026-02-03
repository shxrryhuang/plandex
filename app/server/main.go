package main

import (
	"fmt"
	"log"
	"net/http/pprof"
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

	r := mux.NewRouter()

	if os.Getenv("GOENV") != "production" {
		r.HandleFunc("/debug/pprof/", pprof.Index)
		r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		r.HandleFunc("/debug/pprof/profile", pprof.Profile)
		r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		r.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	routes.AddHealthRoutes(r)
	routes.AddApiRoutes(r)
	routes.AddProxyableApiRoutes(r)
	setup.MustLoadIp()
	setup.MustInitDb()
	setup.StartServer(r, nil, nil)
	os.Exit(0)
}
