package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
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

	lifecycleMu    sync.Mutex
	started        bool
	shutdown       bool
	publicDone     chan struct{}
	managementDone chan struct{}
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
	if s == nil {
		return
	}
	if stop == nil {
		stop = func() {}
	}
	s.lifecycleMu.Lock()
	if s.started || s.shutdown {
		s.lifecycleMu.Unlock()
		return
	}
	s.started = true
	s.publicDone = make(chan struct{})
	if s.management != nil {
		s.managementDone = make(chan struct{})
	}
	publicDone := s.publicDone
	managementDone := s.managementDone
	management := s.management
	public := s.public
	s.lifecycleMu.Unlock()

	if management != nil {
		go func() {
			defer close(managementDone)
			if s.logger != nil {
				s.logger.Info("D-AI management listener started", zap.String("addr", management.Addr))
			}
			if err := management.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				if s.logger != nil {
					s.logger.Error("management listener failed", zap.Error(err))
				}
				stop()
			}
		}()
	}
	go func() {
		defer close(publicDone)
		if s.logger != nil {
			s.logger.Info("D-AI server listening", zap.String("addr", public.Addr), zap.String("version", s.version))
		}
		if err := public.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if s.logger != nil {
				s.logger.Error("server failed", zap.Error(err))
			}
			stop()
		}
	}()
}

func (s *httpServers) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.lifecycleMu.Lock()
	s.shutdown = true
	started := s.started
	public := s.public
	management := s.management
	publicDone := s.publicDone
	managementDone := s.managementDone
	s.lifecycleMu.Unlock()

	var errs []error
	if management != nil {
		shutdownErr := management.Shutdown(ctx)
		if shutdownErr != nil {
			errs = append(errs, fmt.Errorf("management listener: %w", shutdownErr))
		}
		if started {
			if waitErr := waitHTTPServerDone(ctx, managementDone); waitErr != nil && !errors.Is(waitErr, shutdownErr) {
				errs = append(errs, fmt.Errorf("management listener wait: %w", waitErr))
			}
		}
	}
	if public != nil {
		shutdownErr := public.Shutdown(ctx)
		if shutdownErr != nil {
			errs = append(errs, fmt.Errorf("public listener: %w", shutdownErr))
		}
		if started {
			if waitErr := waitHTTPServerDone(ctx, publicDone); waitErr != nil && !errors.Is(waitErr, shutdownErr) {
				errs = append(errs, fmt.Errorf("public listener wait: %w", waitErr))
			}
		}
	}
	return errors.Join(errs...)
}

func waitHTTPServerDone(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
