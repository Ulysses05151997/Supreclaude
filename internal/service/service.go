// Package service runs the HTTP server, optionally under a host service manager
// (Windows service / launchd / systemd) via kardianos/service so the app can
// auto-start on a headless office machine after reboot.
package service

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/kardianos/service"

	"mederos-crm/internal/config"
)

// program implements service.Interface.
type program struct {
	cfg     config.Config
	handler http.Handler
	logger  service.Logger
	srv     *http.Server
}

// Config describes how to construct the service wrapper.
type Config struct {
	AppConfig config.Config
	Handler   http.Handler
}

// New builds a kardianos service for the given handler and app config.
func New(c Config) (service.Service, error) {
	svcConfig := &service.Config{
		Name:        "MederosCRM",
		DisplayName: "Mederos & Associates CRM",
		Description: "Client records server for Mederos & Associates.",
		Arguments:   []string{"run"},
	}
	prg := &program{cfg: c.AppConfig, handler: c.Handler}
	svc, err := service.New(prg, svcConfig)
	if err != nil {
		return nil, err
	}
	logger, err := svc.Logger(nil)
	if err == nil {
		prg.logger = logger
	}
	return svc, nil
}

// Start is called by the service manager; it must not block.
func (p *program) Start(s service.Service) error {
	p.srv = &http.Server{
		Addr:announceAddr(p.cfg.ListenAddr),
		Handler:           p.handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go p.run()
	return nil
}

func (p *program) run() {
	var err error
	if p.cfg.TLSEnabled() {
		p.srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		p.logf("Mederos CRM listening on https://%s", p.cfg.ListenAddr)
		err = p.srv.ListenAndServeTLS(p.cfg.TLSCert, p.cfg.TLSKey)
	} else {
		p.logf("Mederos CRM listening on http://%s", p.cfg.ListenAddr)
		err = p.srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		p.logf("server stopped: %v", err)
	}
}

// Stop is called by the service manager for a graceful shutdown.
func (p *program) Stop(s service.Service) error {
	if p.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return p.srv.Shutdown(ctx)
}

func (p *program) logf(format string, args ...any) {
	if p.logger != nil {
		_ = p.logger.Infof(format, args...)
	}
}

// announceAddr returns the listen address unchanged; kept as a hook in case we
// later want to normalize/validate it.
func announceAddr(addr string) string { return addr }
