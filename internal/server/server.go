package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/fopina/websudo/internal/config"
)

// Server serves proxied requests with placeholder credential validation and upstream auth replacement.
type Server struct {
	cfg        *config.Config
	logger     *slog.Logger
	httpServer *http.Server
}

type matchedRoute struct {
	serviceName string
	variantName string
	service     config.Service
	path        string
}

var errNoConfiguredService = errors.New("no configured service matches request")

// New creates a server from config.
func New(cfg *config.Config) *Server {
	return NewWithLogger(cfg, slog.Default())
}

// NewWithLogger creates a server from config and a logger.
func NewWithLogger(cfg *config.Config, logger *slog.Logger) *Server {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = false
	if cfg.TLS.CAcertPath != "" && cfg.TLS.CAkeyPath != "" {
		if err := applyTLSConfig(proxy, cfg); err != nil {
			panic(err)
		}
	}

	s := &Server{
		cfg:    cfg,
		logger: logger,
	}
	proxy.OnRequest().DoFunc(s.handleRequest)
	proxy.OnResponse().DoFunc(s.handleResponse)
	s.httpServer = &http.Server{
		Addr:    cfg.Listen,
		Handler: proxy,
	}

	return s
}

// Run starts the server.
func (s *Server) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.httpServer.Shutdown(context.Background())
	}()

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (s *Server) handleRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	matched, err := s.matchRoute(req)
	if err != nil {
		if errors.Is(err, errNoConfiguredService) && !s.cfg.BlockUnconfiguredDestinations && req.URL.Hostname() != "" {
			s.logger.Info("request passed through", "method", req.Method, "host", req.URL.Host, "path", req.URL.Path)
			return req, nil
		}
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusForbidden, err.Error())
	}

	if matched.path != req.URL.Path {
		req.URL.Path = matched.path
	}

	if err := validateRequest(req, matched.service); err != nil {
		s.logger.Warn("request denied", "service", matched.serviceName, "variant", matched.variantName, "host", req.URL.Host, "path", req.URL.Path, "error", err)
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusForbidden, err.Error())
	}

	upstreamAuth, err := matched.service.InjectedAuthValue()
	if err != nil {
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, err.Error())
	}

	targetURL, err := url.Parse(matched.service.BaseURL)
	if err != nil {
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, fmt.Sprintf("invalid base_url for %s", matched.serviceName))
	}

	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.Host = targetURL.Host
	req.URL.Path = joinURLPath(targetURL.Path, req.URL.Path)
	req.RequestURI = ""
	req.Header.Set(matched.service.PlaceholderAuth, upstreamAuth)

	ctx.UserData = map[string]string{"service": matched.serviceName, "variant": matched.variantName}
	s.logger.Info("request allowed", "service", matched.serviceName, "variant", matched.variantName, "method", req.Method, "path", req.URL.Path)
	return req, nil
}

func (s *Server) handleResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	if resp != nil && ctx != nil && ctx.UserData != nil {
		s.logger.Info("response proxied", "route", ctx.UserData)
	}
	return resp
}

func (s *Server) matchRoute(req *http.Request) (matchedRoute, error) {
	for name, svc := range s.cfg.Services {
		if svc.RoutePrefix != "" && strings.HasPrefix(req.URL.Path, svc.RoutePrefix) {
			placeholder := req.Header.Get(svc.PlaceholderAuth)
			effective, variantName := svc.EffectiveService(placeholder)
			trimmedPath := strings.TrimPrefix(req.URL.Path, svc.RoutePrefix)
			if trimmedPath == "" {
				trimmedPath = "/"
			}
			return matchedRoute{serviceName: name, variantName: variantName, service: effective, path: trimmedPath}, nil
		}
	}

	host := req.URL.Hostname()
	for name, svc := range s.cfg.Services {
		if !strings.EqualFold(host, svc.MatchHost) {
			continue
		}

		placeholder := req.Header.Get(svc.PlaceholderAuth)
		effective, variantName := svc.EffectiveService(placeholder)
		return matchedRoute{serviceName: name, variantName: variantName, service: effective, path: req.URL.Path}, nil
	}

	return matchedRoute{}, fmt.Errorf("%w host %q or path %q", errNoConfiguredService, host, req.URL.Path)
}

func validateRequest(req *http.Request, svc config.Service) error {
	if len(svc.AllowedMethods) > 0 && !containsFold(svc.AllowedMethods, req.Method) {
		return fmt.Errorf("method %s is not allowed", req.Method)
	}
	for _, denied := range svc.DeniedPaths {
		if pathMatch(denied, req.URL.Path) {
			return fmt.Errorf("path %s is denied", req.URL.Path)
		}
	}
	if len(svc.AllowedPaths) > 0 {
		allowed := false
		for _, candidate := range svc.AllowedPaths {
			if pathMatch(candidate, req.URL.Path) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("path %s is not allowed", req.URL.Path)
		}
	}

	placeholder := req.Header.Get(svc.PlaceholderAuth)
	if placeholder == "" {
		return fmt.Errorf("missing placeholder credentials")
	}
	if !strings.HasPrefix(placeholder, svc.RequirePlaceholderPrefix) {
		return fmt.Errorf("placeholder credentials do not match required prefix")
	}

	return nil
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func pathMatch(pattern string, candidate string) bool {
	ok, err := path.Match(pattern, candidate)
	if err != nil {
		return false
	}
	return ok
}

func joinURLPath(basePath, requestPath string) string {
	if basePath == "" || basePath == "/" {
		return requestPath
	}
	if requestPath == "" || requestPath == "/" {
		return basePath
	}
	return strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(requestPath, "/")
}
