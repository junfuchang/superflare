package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/logger"
)

type daemonRuntime struct {
	newRouter func(*model.Flags) (http.Handler, error)
	listen    func(network string, addr string) (net.Listener, error)
	serve     func(*http.Server, net.Listener) error
	shutdown  func(*http.Server, context.Context) error
}

func StartDaemon(appFlags *model.Flags) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return startDaemonWithContext(ctx, appFlags, daemonRuntime{})
}

func startDaemonWithContext(ctx context.Context, appFlags *model.Flags, runtime daemonRuntime) error {
	log := logger.GetLogger()

	if appFlags == nil {
		return fmt.Errorf("validate daemon flags: app flags cannot be nil")
	}
	effectiveFlags := normalizeRouterFlags(*appFlags)
	if err := validateRouterFlags(effectiveFlags); err != nil {
		return fmt.Errorf("validate daemon flags: %w", err)
	}

	routerBuilder := runtime.newRouter
	if routerBuilder == nil {
		routerBuilder = NewRouter
	}

	router, err := routerBuilder(&effectiveFlags)
	if err != nil {
		log.Error("initialize router failed", "error", err)
		return fmt.Errorf("initialize router: %w", err)
	}

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(effectiveFlags.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	listenFn := runtime.listen
	if listenFn == nil {
		listenFn = net.Listen
	}

	listener, err := listenFn("tcp", srv.Addr)
	if err != nil {
		log.Error("http server listen failed", slog.Any("error", err), "addr", srv.Addr)
		return fmt.Errorf("listen http server: %w", err)
	}

	serveFn := runtime.serve
	if serveFn == nil {
		serveFn = func(srv *http.Server, listener net.Listener) error {
			return srv.Serve(listener)
		}
	}

	shutdownFn := runtime.shutdown
	if shutdownFn == nil {
		shutdownFn = func(srv *http.Server, shutdownCtx context.Context) error {
			return srv.Shutdown(shutdownCtx)
		}
	}

	serveErrCh := make(chan error, 1)
	go func() {
		if err := serveFn(srv, listener); err != nil && err != http.ErrServerClosed {
			serveErrCh <- err
			return
		}
		close(serveErrCh)
	}()

	log.Info("http server started", "addr", srv.Addr)

	select {
	case err, ok := <-serveErrCh:
		if ok && err != nil {
			log.Error("http server exited with error", slog.Any("error", err))
			return fmt.Errorf("serve http server: %w", err)
		}
	case <-ctx.Done():
	}

	log.Info("shutting down http server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := shutdownFn(srv, shutdownCtx); err != nil {
		log.Error("http server shutdown failed", slog.Any("error", err))
		return fmt.Errorf("shutdown http server: %w", err)
	}

	log.Info("http server stopped")
	return nil
}
