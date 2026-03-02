package reserverpc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

type Options struct {
	Bind string
	Port int
}

type Server struct {
	addr string
	mux  *http.ServeMux
	srv  *http.Server
	ln   net.Listener
}

func New(opt Options) *Server {
	addr := fmt.Sprintf("%s:%d", opt.Bind, opt.Port)
	mux := http.NewServeMux()
	return &Server{addr: addr, mux: mux}
}

func (s *Server) HandleFunc(path string, h func(http.ResponseWriter, *http.Request)) {
	s.mux.HandleFunc(path, h)
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.srv = &http.Server{
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go s.srv.Serve(ln)
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}
