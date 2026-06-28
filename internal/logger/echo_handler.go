package logger

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v5"
)

const requestAttrsCap = 12
const requestLogQueueSize = 256

var requestAttrsPool = sync.Pool{
	New: func() any {
		attrs := make([]slog.Attr, 0, requestAttrsCap)
		return &attrs
	},
}

type requestLogEntry struct {
	logger *slog.Logger
	level  slog.Level
	msg    string
	attrs  *[]slog.Attr
}

type requestLogDispatcher struct {
	once    sync.Once
	jobs    chan requestLogEntry
	limit   int32
	pending atomic.Int32
}

func newRequestLogDispatcher(size int) *requestLogDispatcher {
	if size <= 0 {
		size = 1
	}
	return &requestLogDispatcher{
		jobs:  make(chan requestLogEntry, size),
		limit: int32(size),
	}
}

func (d *requestLogDispatcher) start() {
	if d == nil {
		return
	}
	d.once.Do(func() {
		go func() {
			for job := range d.jobs {
				if job.logger != nil {
					job.logger.LogAttrs(context.Background(), job.level, job.msg, (*job.attrs)...)
				}
				releaseRequestAttrs(job.attrs)
				d.releaseSlot()
			}
		}()
	})
}

func (d *requestLogDispatcher) queueLimit() int32 {
	if d == nil {
		return 0
	}
	if d.limit > 0 {
		return d.limit
	}
	return int32(cap(d.jobs))
}

func (d *requestLogDispatcher) tryAcquireSlot() bool {
	limit := d.queueLimit()
	if limit <= 0 {
		return false
	}
	for {
		current := d.pending.Load()
		if current >= limit {
			return false
		}
		if d.pending.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (d *requestLogDispatcher) releaseSlot() {
	if d == nil {
		return
	}
	for {
		current := d.pending.Load()
		if current <= 0 {
			return
		}
		if d.pending.CompareAndSwap(current, current-1) {
			return
		}
	}
}

func (d *requestLogDispatcher) submit(logger *slog.Logger, level slog.Level, msg string, attrs *[]slog.Attr) bool {
	if d == nil {
		return false
	}
	if !d.tryAcquireSlot() {
		return false
	}
	select {
	case d.jobs <- requestLogEntry{
		logger: logger,
		level:  level,
		msg:    msg,
		attrs:  attrs,
	}:
		d.start()
		return true
	default:
		d.releaseSlot()
		return false
	}
}

func releaseRequestAttrs(attrs *[]slog.Attr) {
	if attrs == nil {
		return
	}
	*attrs = (*attrs)[:0]
	requestAttrsPool.Put(attrs)
}

var asyncRequestLogDispatcher = newRequestLogDispatcher(requestLogQueueSize)

// LoggerConfig configures the request logging middleware.
type LoggerConfig struct {
	// Skipper returns true to skip logging for the request (e.g. health check, favicon).
	Skipper func(c *echo.Context) bool
}

// NewEcho returns an Echo middleware that logs each request with slog.
func NewEcho(logger *slog.Logger) echo.MiddlewareFunc {
	return NewEchoWithConfig(logger, LoggerConfig{})
}

// NewEchoWithConfig returns the middleware with optional skipper and tuning.
func NewEchoWithConfig(logger *slog.Logger, config LoggerConfig) echo.MiddlewareFunc {
	skipper := config.Skipper
	if skipper == nil {
		skipper = func(*echo.Context) bool { return false }
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if skipper(c) {
				return next(c)
			}
			start := time.Now()
			path := c.Request().URL.Path
			query := c.Request().URL.RawQuery

			err := next(c)

			status := 0
			if rw, unwrapErr := echo.UnwrapResponse(c.Response()); unwrapErr == nil && rw != nil {
				status = rw.Status
			}
			// Only build path params for error responses to keep 2xx hot path allocation-free.
			var params map[string]string
			if status >= http.StatusBadRequest {
				if pv := c.PathValues(); len(pv) > 0 {
					params = make(map[string]string, len(pv))
					for _, v := range pv {
						params[v.Name] = v.Value
					}
				}
			}
			method := c.Request().Method
			host := c.Request().Host
			route := c.Path()
			latency := time.Since(start)
			userAgent := c.Request().UserAgent()
			ip := c.RealIP()
			referer := c.Request().Referer()

			attrsPtr, ok := requestAttrsPool.Get().(*[]slog.Attr)
			if !ok || attrsPtr == nil {
				attrsPtr = &[]slog.Attr{}
			}
			requestAttributes := attrsPtr
			*requestAttributes = (*requestAttributes)[:0]
			*requestAttributes = append(*requestAttributes,
				slog.Time("time", start),
				slog.String("method", method),
				slog.String("host", host),
				slog.String("path", path),
				slog.String("query", query),
			)
			if params != nil {
				*requestAttributes = append(*requestAttributes, slog.Any("params", params))
			}
			*requestAttributes = append(*requestAttributes,
				slog.String("route", route),
				slog.String("ip", ip),
				slog.String("referer", referer),
				slog.String("user-agent", userAgent),
				slog.Duration("latency", latency),
				slog.Int("status", status),
			)

			level := slog.LevelInfo
			msg := "REQUEST"
			if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
				level = slog.LevelWarn
				if err != nil {
					msg = err.Error()
				}
			} else if status >= http.StatusInternalServerError {
				level = slog.LevelError
				if err != nil {
					msg = err.Error()
				}
			}

			// Async log for 2xx to shorten critical path and improve throughput.
			if status >= 200 && status < 300 {
				if !asyncRequestLogDispatcher.submit(logger, level, msg, requestAttributes) {
					logger.LogAttrs(context.Background(), level, msg, (*requestAttributes)...)
					releaseRequestAttrs(requestAttributes)
				}
			} else {
				logger.LogAttrs(c.Request().Context(), level, msg, (*requestAttributes)...)
				releaseRequestAttrs(requestAttributes)
			}
			return err
		}
	}
}

// DefaultRequestLogSkipper skips logging for health check, favicon, redirects, and static assets to reduce allocs in hot paths.
func DefaultRequestLogSkipper(c *echo.Context) bool {
	path := c.Request().URL.Path
	return path == "/ping" || path == "/favicon.ico" || strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/redir/")
}
