package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type httpServerOptions struct {
	PublicAddr        string
	ManagementAddr    string
	PublicHandler     http.Handler
	ManagementHandler http.Handler
	ReadTimeout       time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	Logger            *zap.Logger
	Version           string
}

type httpServers struct {
	public     *http.Server
	management *http.Server
	logger     *zap.Logger
	version    string
}

func newHTTPServers(opts httpServerOptions) *httpServers {
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	servers := &httpServers{
		public: &http.Server{
			Addr:              opts.PublicAddr,
			Handler:           opts.PublicHandler,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       opts.ReadTimeout,
			// Streaming AI responses use application-level response-header,
			// first-byte, idle-gap and max-duration deadlines in serving; a
			// server write timeout would terminate healthy long-lived streams.
			WriteTimeout:   0,
			IdleTimeout:    opts.IdleTimeout,
			MaxHeaderBytes: opts.MaxHeaderBytes,
		},
		logger:  opts.Logger,
		version: opts.Version,
	}
	if opts.ManagementAddr != "" {
		servers.management = &http.Server{
			Addr:              opts.ManagementAddr,
			Handler:           opts.ManagementHandler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       30 * time.Second,
			MaxHeaderBytes:    opts.MaxHeaderBytes,
		}
	}
	return servers
}

func (s *httpServers) Start(stop context.CancelFunc) {
	if stop == nil {
		stop = func() {}
	}
	if s.management != nil {
		go func() {
			s.logger.Info("D-AI management listener started", zap.String("addr", s.management.Addr))
			if err := s.management.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.logger.Error("management listener failed", zap.Error(err))
				stop()
			}
		}()
	}
	go func() {
		s.logger.Info("D-AI server listening", zap.String("addr", s.public.Addr), zap.String("version", s.version))
		if err := s.public.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("server failed", zap.Error(err))
			stop()
		}
	}()
}

func (s *httpServers) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.management != nil {
		if err := s.management.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("management listener: %w", err))
		}
	}
	if s.public != nil {
		if err := s.public.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("public listener: %w", err))
		}
	}
	return errors.Join(errs...)
}
