package logger

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

type stubLogHandler struct {
	active int32
	peak   int32
	delay  time.Duration
	count  int32
}

type countOnlyHandler struct {
	count *atomic.Int32
}

func (h *stubLogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *stubLogHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *stubLogHandler) WithGroup(string) slog.Handler            { return h }

func (h *stubLogHandler) Handle(ctx context.Context, r slog.Record) error {
	current := atomic.AddInt32(&h.active, 1)
	for {
		prev := atomic.LoadInt32(&h.peak)
		if current <= prev || atomic.CompareAndSwapInt32(&h.peak, prev, current) {
			break
		}
	}
	atomic.AddInt32(&h.count, 1)
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	atomic.AddInt32(&h.active, -1)
	return nil
}

func (h *countOnlyHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countOnlyHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *countOnlyHandler) WithGroup(string) slog.Handler            { return h }
func (h *countOnlyHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.count != nil {
		h.count.Add(1)
	}
	return nil
}

func TestRequestLogDispatcherLimitsConcurrentAsyncLogs(t *testing.T) {
	dispatcher := newRequestLogDispatcher(64)
	handler := &stubLogHandler{delay: 15 * time.Millisecond}
	logger := slog.New(handler)

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			attrs := requestAttrsPool.Get().(*[]slog.Attr)
			*attrs = (*attrs)[:0]
			*attrs = append(*attrs, slog.String("path", "/"))
			if !dispatcher.submit(logger, slog.LevelInfo, "REQUEST", attrs) {
				releaseRequestAttrs(attrs)
				errCh <- context.DeadlineExceeded
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal("unexpected dispatcher fallback during concurrency test")
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&handler.count) == 32 && atomic.LoadInt32(&handler.active) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if atomic.LoadInt32(&handler.count) != 32 {
		t.Fatalf("expected 32 log entries, got %d", atomic.LoadInt32(&handler.count))
	}
	if atomic.LoadInt32(&handler.peak) != 1 {
		t.Fatalf("expected single dispatcher goroutine, got peak concurrency %d", atomic.LoadInt32(&handler.peak))
	}
}

func TestRequestLogDispatcherSubmitFallsBackWhenQueueFull(t *testing.T) {
	dispatcher := newRequestLogDispatcher(1)
	handler := &stubLogHandler{delay: 80 * time.Millisecond}
	logger := slog.New(handler)

	first := requestAttrsPool.Get().(*[]slog.Attr)
	*first = (*first)[:0]
	*first = append(*first, slog.String("path", "/first"))
	if !dispatcher.submit(logger, slog.LevelInfo, "REQUEST", first) {
		t.Fatal("expected first enqueue to succeed")
	}

	second := requestAttrsPool.Get().(*[]slog.Attr)
	*second = (*second)[:0]
	*second = append(*second, slog.String("path", "/second"))
	if dispatcher.submit(logger, slog.LevelInfo, "REQUEST", second) {
		t.Fatal("expected queue-full submit to fail")
	}
	releaseRequestAttrs(second)
}

func TestNewEchoWithConfigLogsSuccessPathWithoutLeakingAttrs(t *testing.T) {
	origDispatcher := asyncRequestLogDispatcher
	asyncRequestLogDispatcher = newRequestLogDispatcher(16)
	defer func() { asyncRequestLogDispatcher = origDispatcher }()

	handler := &stubLogHandler{}
	log := slog.New(handler)

	e := echo.New()
	e.Use(NewEchoWithConfig(log, LoggerConfig{}))
	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&handler.count) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected one success log entry, got %d", atomic.LoadInt32(&handler.count))
}

func TestNewEchoWithConfigQueueFullFallsBackToSyncLog(t *testing.T) {
	origDispatcher := asyncRequestLogDispatcher
	asyncRequestLogDispatcher = &requestLogDispatcher{jobs: make(chan requestLogEntry)}
	defer func() { asyncRequestLogDispatcher = origDispatcher }()

	var count atomic.Int32
	log := slog.New(&countOnlyHandler{count: &count})

	e := echo.New()
	e.Use(NewEchoWithConfig(log, LoggerConfig{}))
	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if count.Load() != 1 {
		t.Fatalf("expected sync fallback log, got %d", count.Load())
	}
}
