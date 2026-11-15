// Package server runs the proxies of the configuration file and applies its changes while running.
package server

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/MatthieuSarter/socquette/internal/auth"
	"github.com/MatthieuSarter/socquette/internal/config"
	"github.com/MatthieuSarter/socquette/internal/httpproxy"
	"github.com/MatthieuSarter/socquette/internal/socks5"
)

// Listeners are the listen addresses of the two proxies; "" means not listening.
type Listeners struct {
	HTTP  string
	SOCKS string
}

// Wanted returns the addresses a configuration asks the proxies to listen on.
func Wanted(c config.Config) Listeners {
	var w Listeners
	if c.HTTP.Enabled {
		w.HTTP = c.HTTP.Addr()
	}
	if c.SOCKS.Enabled {
		w.SOCKS = c.SOCKS.Addr()
	}
	return w
}

// fileStamp identifies a version of the configuration file; the zero value means "missing".
type fileStamp struct {
	mod  time.Time
	size int64
}

func stat(path string) (fileStamp, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileStamp{}, nil
	}
	if err != nil {
		return fileStamp{}, err
	}
	return fileStamp{mod: fi.ModTime(), size: fi.Size()}, nil
}

type runner struct {
	paths config.Paths
	log   *slog.Logger

	auth      atomic.Pointer[auth.Authenticator]
	advertise atomic.Pointer[string]

	stamp, badStamp fileStamp
	statErr         string
	cfg             config.Config
	http, socks     slot
	bound           Listeners
}

func newRunner(paths config.Paths, log *slog.Logger) *runner {
	r := &runner{paths: paths, log: log}
	r.apply(config.Defaults())
	socksServer := &socks5.Server{
		Auth:      r.auth.Load,
		Advertise: func() string { return *r.advertise.Load() },
		Log:       log,
	}
	handler := &httpproxy.Handler{Auth: r.auth.Load, Log: log}
	r.http = slot{name: "http", bind: newHTTPBinder(handler)}
	r.socks = slot{name: "socks", bind: socksBinder(socksServer)}
	return r
}

func (r *runner) apply(c config.Config) {
	r.cfg = c
	r.auth.Store(auth.New(c))
	adv := c.SOCKS.AdvertiseAddress
	r.advertise.Store(&adv)
}

// tick reloads the configuration file when it changed, then brings the listeners in line with it.
func (r *runner) tick() {
	st, err := stat(r.paths.Config)
	if err != nil {
		if err.Error() != r.statErr {
			r.log.Error("cannot read the configuration file", "error", err)
			r.statErr = err.Error()
		}
		return
	}
	r.statErr = ""
	if st != r.stamp {
		r.reload(st)
	}
	w := Wanted(r.cfg)
	r.reconcile(&r.http, w.HTTP)
	r.reconcile(&r.socks, w.SOCKS)
	r.bound = Listeners{HTTP: r.http.addr, SOCKS: r.socks.addr}
}

func (r *runner) reload(st fileStamp) {
	c, _, err := config.Load(r.paths.Config)
	if err != nil {
		if st != r.badStamp {
			r.log.Error("invalid configuration, keeping the previous one", "error", err)
			r.badStamp = st
		}
		return
	}
	r.stamp, r.badStamp = st, fileStamp{}
	r.apply(c)
	w := Wanted(c)
	r.log.Info("configuration loaded", "http", w.HTTP, "socks", w.SOCKS, "users", len(c.Users))
}

func (r *runner) close() {
	r.http.shutdown()
	r.socks.shutdown()
	r.bound = Listeners{}
}

// Run serves the proxies of the configuration in paths, polling the file every poll, until ctx is done.
func Run(ctx context.Context, paths config.Paths, log *slog.Logger, poll time.Duration) error {
	r := newRunner(paths, log)
	defer r.close()
	log.Info("server starting", "config", paths.Config)
	r.tick()
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("server stopping")
			return nil
		case <-t.C:
			r.tick()
		}
	}
}
