package server

import (
	"context"
	"errors"
	"fmt"
	"io"
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

type authTarget struct {
	kind string
	name string
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
	proxy.NonproxyHandler = http.HandlerFunc(s.handleNonProxyRequest)
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

func (s *Server) handleNonProxyRequest(w http.ResponseWriter, req *http.Request) {
	ctx := &goproxy.ProxyCtx{}
	outReq, resp := s.handleRequest(req, ctx)
	if resp == nil {
		var err error
		resp, err = http.DefaultTransport.RoundTrip(outReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	resp = s.handleResponse(resp, ctx)
	writeResponse(w, resp)
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

	placeholderTarget, err := parseAuthTarget(matched.service.PlaceholderAuth)
	if err != nil {
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, err.Error())
	}
	injectTargetRaw := matched.service.InjectAuthTarget
	if injectTargetRaw == "" {
		injectTargetRaw = matched.service.PlaceholderAuth
	}
	injectTarget, err := parseAuthTarget(injectTargetRaw)
	if err != nil {
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, err.Error())
	}

	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.Host = targetURL.Host
	req.URL.Path = joinURLPath(targetURL.Path, req.URL.Path)
	req.RequestURI = ""
	clearAuthValue(req, placeholderTarget)
	setAuthValue(req, injectTarget, upstreamAuth)

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

func writeResponse(w http.ResponseWriter, resp *http.Response) {
	if resp == nil {
		http.Error(w, "empty upstream response", http.StatusBadGateway)
		return
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		_, _ = io.Copy(w, resp.Body)
	}
}

func (s *Server) matchRoute(req *http.Request) (matchedRoute, error) {
	for name, svc := range s.cfg.Services {
		if svc.RoutePrefix != "" && strings.HasPrefix(req.URL.Path, svc.RoutePrefix) {
			placeholder, err := getAuthValue(req, svc.PlaceholderAuth)
			if err != nil {
				return matchedRoute{}, err
			}
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

		placeholder, err := getAuthValue(req, svc.PlaceholderAuth)
		if err != nil {
			return matchedRoute{}, err
		}
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

	placeholder, err := getAuthValue(req, svc.PlaceholderAuth)
	if err != nil {
		return err
	}
	if placeholder == "" {
		return fmt.Errorf("missing placeholder credentials")
	}
	if !strings.HasPrefix(placeholder, svc.RequirePlaceholderPrefix) {
		return fmt.Errorf("placeholder credentials do not match required prefix")
	}

	return nil
}

func getAuthValue(req *http.Request, rawTarget string) (string, error) {
	target, err := parseAuthTarget(rawTarget)
	if err != nil {
		return "", err
	}

	switch target.kind {
	case "header":
		return req.Header.Get(target.name), nil
	case "cookie":
		cookie, err := req.Cookie(target.name)
		if errors.Is(err, http.ErrNoCookie) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		return cookie.Value, nil
	default:
		return "", fmt.Errorf("unsupported auth target kind %q", target.kind)
	}
}

func setAuthValue(req *http.Request, target authTarget, value string) {
	switch target.kind {
	case "header":
		req.Header.Set(target.name, value)
	case "cookie":
		setCookie(req, target.name, value)
	}
}

func clearAuthValue(req *http.Request, target authTarget) {
	switch target.kind {
	case "header":
		req.Header.Del(target.name)
	case "cookie":
		deleteCookie(req, target.name)
	}
}

func parseAuthTarget(raw string) (authTarget, error) {
	if raw == "" {
		return authTarget{}, fmt.Errorf("auth target cannot be empty")
	}
	if !strings.Contains(raw, ":") {
		return authTarget{kind: "header", name: raw}, nil
	}

	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return authTarget{}, fmt.Errorf("invalid auth target %q", raw)
	}
	kind := strings.ToLower(parts[0])
	if kind != "header" && kind != "cookie" {
		return authTarget{}, fmt.Errorf("unsupported auth target %q", raw)
	}
	return authTarget{kind: kind, name: parts[1]}, nil
}

func setCookie(req *http.Request, name string, value string) {
	cookies := req.Cookies()
	updated := false
	parts := make([]string, 0, len(cookies)+1)
	for _, cookie := range cookies {
		if cookie.Name == name {
			cookie.Value = value
			updated = true
		}
		parts = append(parts, cookie.String())
	}
	if !updated {
		parts = append(parts, (&http.Cookie{Name: name, Value: value}).String())
	}
	if len(parts) == 0 {
		req.Header.Del("Cookie")
		return
	}
	req.Header.Set("Cookie", strings.Join(parts, "; "))
}

func deleteCookie(req *http.Request, name string) {
	cookies := req.Cookies()
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Name == name {
			continue
		}
		parts = append(parts, cookie.String())
	}
	if len(parts) == 0 {
		req.Header.Del("Cookie")
		return
	}
	req.Header.Set("Cookie", strings.Join(parts, "; "))
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
