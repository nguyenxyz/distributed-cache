package apiserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/ph-ngn/nanobox/telemetry"
)

type Server struct {
	sv     *http.Server
	router *chi.Mux
	cc     *ClusterController
}

func NewServer(cc *ClusterController) (*Server, error) {
	if cc == nil {
		return nil, errors.New("cluster controller must be provided")
	}
	server := &Server{
		sv: &http.Server{
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  15 * time.Second,
		},
		cc: cc,
	}

	server.setupRouter()
	server.setupMiddleware()

	return server, nil
}

func (s *Server) setupRouter() {
	s.router = chi.NewMux()
	s.router.Get("/GET/{key}", s.cc.Get)
	s.router.Get("/PEEK/{key}", s.cc.Peek)
	s.router.Put("/SET/{key}", s.cc.Set)
	s.router.Patch("/UPDATE/{key}", s.cc.Update)
	s.router.Delete("/DELETE/{key}", s.cc.Delete)
	s.router.Delete("/PURGE", s.cc.Purge)
	s.router.Get("/KEYS", s.cc.Keys)
	s.router.Get("/ENTRIES", s.cc.Entries)
	s.router.Get("/SIZE", s.cc.Size)
	s.router.Get("/CAP", s.cc.Cap)
	s.router.Patch("/RESIZE", s.cc.Resize)
}

func (s *Server) setupMiddleware() {
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.sv.Handler.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	s.sv.Addr = addr
	s.sv.Handler = s.router
	telemetry.Log().Infof("Starting API server on %s", addr)
	return s.sv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	telemetry.Log().Infof("Shutting down API server")
	return s.sv.Shutdown(ctx)
}
