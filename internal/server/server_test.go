package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/junfuchang/superflare/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartDaemonWithContext_ReturnsRouterInitError(t *testing.T) {
	wantErr := errors.New("router init failed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := startDaemonWithContext(ctx, &model.Flags{Port: 3636, DisableLoginMode: true}, daemonRuntime{
		newRouter: func(*model.Flags) (http.Handler, error) {
			return nil, wantErr
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Contains(t, err.Error(), "initialize router")
}

func TestStartDaemonWithContext_ReturnsErrorWhenFlagsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := startDaemonWithContext(ctx, nil, daemonRuntime{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "app flags cannot be nil")
}

func TestStartDaemonWithContext_ReturnsErrorWhenPortInvalid(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := startDaemonWithContext(ctx, &model.Flags{Port: 0, DisableLoginMode: true}, daemonRuntime{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid port 0")
}

func TestStartDaemonWithContext_ReturnsErrorWhenLoginCredentialsIncomplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := startDaemonWithContext(ctx, &model.Flags{
		Port:             3636,
		Visibility:       "DEFAULT",
		DisableLoginMode: false,
		User:             "admin",
		Pass:             "",
		CookieName:       "superflare",
		CookieSecret:     "secret",
	}, daemonRuntime{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate daemon flags")
	assert.Contains(t, err.Error(), "login credentials")
}

func TestStartDaemonWithContext_NormalizesLowercaseVisibility(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- startDaemonWithContext(ctx, &model.Flags{
			Port:             3636,
			Visibility:       "private",
			DisableLoginMode: true,
		}, daemonRuntime{
			newRouter: func(flags *model.Flags) (http.Handler, error) {
				if flags == nil {
					t.Fatal("expected normalized flags")
				}
				if flags.Visibility != "PRIVATE" {
					t.Fatalf("expected PRIVATE visibility, got %q", flags.Visibility)
				}
				return http.NewServeMux(), nil
			},
			listen: func(network string, addr string) (net.Listener, error) {
				return dummyListener{}, nil
			},
			serve: func(*http.Server, net.Listener) error {
				<-ctx.Done()
				return http.ErrServerClosed
			},
			shutdown: func(*http.Server, context.Context) error {
				return nil
			},
		})
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-errCh
	require.NoError(t, err)
}

func TestStartDaemonWithContext_ReturnsServeError(t *testing.T) {
	wantErr := errors.New("listen failed")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := startDaemonWithContext(ctx, &model.Flags{Port: 3636, DisableLoginMode: true}, daemonRuntime{
		newRouter: func(*model.Flags) (http.Handler, error) {
			return http.NewServeMux(), nil
		},
		listen: func(network string, addr string) (net.Listener, error) {
			return dummyListener{}, nil
		},
		serve: func(*http.Server, net.Listener) error {
			return wantErr
		},
		shutdown: func(*http.Server, context.Context) error {
			return nil
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Contains(t, err.Error(), "serve http server")
}

func TestStartDaemonWithContext_ReturnsShutdownError(t *testing.T) {
	wantErr := errors.New("shutdown failed")
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- startDaemonWithContext(ctx, &model.Flags{Port: 3636, DisableLoginMode: true}, daemonRuntime{
			newRouter: func(*model.Flags) (http.Handler, error) {
				return http.NewServeMux(), nil
			},
			listen: func(network string, addr string) (net.Listener, error) {
				return dummyListener{}, nil
			},
			serve: func(*http.Server, net.Listener) error {
				<-ctx.Done()
				return http.ErrServerClosed
			},
			shutdown: func(*http.Server, context.Context) error {
				return wantErr
			},
		})
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-errCh
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Contains(t, err.Error(), "shutdown http server")
}

func TestStartDaemonWithContext_ReturnsListenError(t *testing.T) {
	wantErr := errors.New("bind failed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := startDaemonWithContext(ctx, &model.Flags{Port: 3636, DisableLoginMode: true}, daemonRuntime{
		newRouter: func(*model.Flags) (http.Handler, error) {
			return http.NewServeMux(), nil
		},
		listen: func(network string, addr string) (net.Listener, error) {
			return nil, wantErr
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Contains(t, err.Error(), "listen http server")
}

type dummyListener struct{}

func (dummyListener) Accept() (net.Conn, error) { return nil, http.ErrServerClosed }
func (dummyListener) Close() error              { return nil }
func (dummyListener) Addr() net.Addr            { return dummyAddr("127.0.0.1:3636") }

type dummyAddr string

func (a dummyAddr) Network() string { return "tcp" }
func (a dummyAddr) String() string  { return string(a) }
