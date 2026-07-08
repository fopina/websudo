package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
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
	requestPath string
}

const builtinCACertPath = "/.well-known/websudo/ca.pem"

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
		if err := applyTLSConfig(proxy, cfg, logger); err != nil {
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
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	s.logStartup(listener)

	go func() {
		<-ctx.Done()
		_ = s.httpServer.Shutdown(context.Background())
	}()

	err = s.httpServer.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (s *Server) logStartup(listener net.Listener) {
	var services map[string]config.Service
	if s.cfg != nil {
		services = s.cfg.Services
	}

	s.log().Info(
		"proxy listening",
		"configured_listen", s.httpServer.Addr,
		"addresses", listenerAddresses(listener),
		"ports", listenerPorts(listener),
		"services", serviceConfigSummaries(services),
	)
}

func (s *Server) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

func listenerAddresses(listener net.Listener) []string {
	if listener == nil || listener.Addr() == nil {
		return nil
	}
	return []string{listener.Addr().String()}
}

func listenerPorts(listener net.Listener) []string {
	if listener == nil || listener.Addr() == nil {
		return nil
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return nil
	}
	return []string{port}
}

func serviceConfigSummaries(services map[string]config.Service) []string {
	names := sortedServiceNames(services)
	summaries := make([]string, 0, len(names))
	for _, name := range names {
		svc := services[name]
		summaries = append(summaries, fmt.Sprintf("%s base_url=%s modes=%s", name, svc.BaseURL, strings.Join(serviceModes(svc), ",")))
	}
	return summaries
}

func sortedServiceNames(services map[string]config.Service) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func serviceModes(svc config.Service) []string {
	modes := make([]string, 0, 2)
	if svc.MatchHost != "" {
		modes = append(modes, fmt.Sprintf("forward(match_host=%s)", svc.MatchHost))
	}
	if svc.RoutePrefix != "" {
		modes = append(modes, fmt.Sprintf("reverse(route_prefix=%s)", svc.RoutePrefix))
	}
	if len(modes) == 0 {
		return []string{"unmatched"}
	}
	return modes
}

func (s *Server) handleNonProxyRequest(w http.ResponseWriter, req *http.Request) {
	if s.handleBuiltinRequest(w, req) {
		return
	}

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
	if resp := s.builtinResponse(req); resp != nil {
		return req, resp
	}

	matched, err := s.matchRoute(req)
	if err != nil {
		if errors.Is(err, errNoConfiguredService) && req.URL.Hostname() != "" {
			if !s.cfg.BlockUnconfiguredDestinations {
				s.log().Info("request passed through", "method", req.Method, "host", req.URL.Host, "path", req.URL.Path)
				return req, nil
			}
			s.log().Info("request blocked unconfigured", "method", req.Method, "host", req.URL.Host, "path", req.URL.Path)
		}
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusForbidden, err.Error())
	}

	if matched.path != req.URL.Path {
		req.URL.Path = matched.path
	}
	if err := decryptRequestCookies(req, matched.service); err != nil {
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, err.Error())
	}
	if err := decryptRequestAuthToken(req, matched.service); err != nil {
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, err.Error())
	}

	targetURL, err := url.Parse(matched.service.BaseURL)
	if err != nil {
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, fmt.Sprintf("invalid base_url for %s", matched.serviceName))
	}

	var placeholderHeader string
	if matched.service.PlaceholderAuth != "" {
		parsedPlaceholderHeader, err := parseHeaderAuthTarget(matched.service.PlaceholderAuth)
		if err != nil {
			return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, err.Error())
		}
		placeholderHeader = parsedPlaceholderHeader
	}
	var injectHeader string
	if matched.service.InjectAuth != "" {
		injectTargetRaw := matched.service.InjectAuthTarget
		if injectTargetRaw == "" {
			injectTargetRaw = matched.service.PlaceholderAuth
		}
		parsedInjectHeader, err := parseHeaderAuthTarget(injectTargetRaw)
		if err != nil {
			return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, err.Error())
		}
		injectHeader = parsedInjectHeader
	}

	isLogin := isLoginRequest(req, matched.service)
	if isLogin {
		if err := rewriteLoginRequest(req, matched.service); err != nil {
			if errors.Is(err, errLoginPlaceholderCredentials) {
				return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusForbidden, err.Error())
			}
			return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusBadRequest, err.Error())
		}
		if placeholderHeader != "" {
			req.Header.Del(placeholderHeader)
		}
	} else {
		if err := validateRequest(req, matched.service); err != nil {
			s.log().Warn("request denied", "service", matched.serviceName, "variant", matched.variantName, "host", req.URL.Host, "requested_path", matched.requestPath, "upstream_path", req.URL.Path, "error", err)
			return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusForbidden, validationErrorMessage(err, matched))
		}

		if matched.service.InjectAuth != "" && matched.service.Login.Path == "" {
			upstreamAuth, err := matched.service.InjectedAuthValue()
			if err != nil {
				return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, err.Error())
			}
			if placeholderHeader != "" {
				req.Header.Del(placeholderHeader)
			}
			if injectHeader != "" {
				req.Header.Set(injectHeader, upstreamAuth)
			}
		}
	}

	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.Host = targetURL.Host
	req.URL.Path = joinURLPath(targetURL.Path, req.URL.Path)
	req.RequestURI = ""

	ctx.UserData = routeContext{serviceName: matched.serviceName, variantName: matched.variantName, service: matched.service, isLogin: isLogin}
	s.log().Info("request allowed", "service", matched.serviceName, "variant", matched.variantName, "method", req.Method, "path", req.URL.Path, "login", isLogin)
	return req, nil
}

func (s *Server) handleResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	if resp == nil {
		return nil
	}
	if ctx != nil {
		if route, ok := ctx.UserData.(routeContext); ok {
			if err := encryptResponseCookies(resp, route.service); err != nil {
				s.log().Warn("response cookie encryption failed", "service", route.serviceName, "variant", route.variantName, "error", err)
				return responseError(resp, resp.Request, http.StatusInternalServerError, err.Error())
			}
			if route.isLogin {
				if err := encryptLoginResponseAuthToken(resp, route.service); err != nil {
					s.log().Warn("response token encryption failed", "service", route.serviceName, "variant", route.variantName, "error", err)
					return responseError(resp, resp.Request, http.StatusInternalServerError, err.Error())
				}
			}
			s.log().Info("response proxied", "service", route.serviceName, "variant", route.variantName)
		}
	}
	return resp
}

func (s *Server) handleBuiltinRequest(w http.ResponseWriter, req *http.Request) bool {
	resp := s.builtinResponse(req)
	if resp == nil {
		return false
	}
	writeResponse(w, resp)
	return true
}

func (s *Server) builtinResponse(req *http.Request) *http.Response {
	if req.URL.Path != builtinCACertPath {
		return nil
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		resp := goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusMethodNotAllowed, "method not allowed")
		resp.Header.Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
		return resp
	}
	if s.cfg.TLS.CAcertPath == "" {
		return goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusNotFound, "CA certificate is not configured")
	}

	certPEM, err := os.ReadFile(s.cfg.TLS.CAcertPath)
	if err != nil {
		return goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, fmt.Sprintf("read CA cert: %v", err))
	}

	body := io.NopCloser(bytes.NewReader(certPEM))
	if req.Method == http.MethodHead {
		body = http.NoBody
	}

	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)),
		Header:        caCertDownloadHeaders(len(certPEM)),
		Body:          body,
		ContentLength: int64(len(certPEM)),
		Request:       req,
	}
}

func caCertDownloadHeaders(contentLength int) http.Header {
	header := make(http.Header)
	header.Set("Content-Type", "application/x-pem-file")
	header.Set("Content-Disposition", `attachment; filename="websudo-ca.pem"`)
	header.Set("Content-Length", fmt.Sprintf("%d", contentLength))
	header.Set("Cache-Control", "no-store")
	return header
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
	host := req.URL.Hostname()
	for _, name := range sortedServiceNames(s.cfg.Services) {
		svc := s.cfg.Services[name]
		if svc.MatchHost == "" || !strings.EqualFold(host, svc.MatchHost) {
			continue
		}

		placeholder, err := getAuthValue(req, svc.PlaceholderAuth)
		if err != nil {
			return matchedRoute{}, err
		}
		effective, variantName := svc.EffectiveService(placeholder)
		return matchedRoute{serviceName: name, variantName: variantName, service: effective, path: req.URL.Path, requestPath: req.URL.Path}, nil
	}

	for _, name := range sortedServiceNames(s.cfg.Services) {
		svc := s.cfg.Services[name]
		if svc.RoutePrefix != "" && routePrefixMatches(req.URL.Path, svc.RoutePrefix) {
			placeholder, err := getAuthValue(req, svc.PlaceholderAuth)
			if err != nil {
				return matchedRoute{}, err
			}
			effective, variantName := svc.EffectiveService(placeholder)
			trimmedPath := trimRoutePrefix(req.URL.Path, svc.RoutePrefix)
			return matchedRoute{serviceName: name, variantName: variantName, service: effective, path: trimmedPath, requestPath: req.URL.Path}, nil
		}
	}

	return matchedRoute{}, fmt.Errorf("%w host %q or path %q", errNoConfiguredService, host, req.URL.Path)
}

func routePrefixMatches(requestPath string, routePrefix string) bool {
	prefix := normalizedRoutePrefix(routePrefix)
	return requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") || prefix == "/"
}

func trimRoutePrefix(requestPath string, routePrefix string) string {
	prefix := normalizedRoutePrefix(routePrefix)
	if prefix == "/" {
		return requestPath
	}
	trimmedPath := strings.TrimPrefix(requestPath, prefix)
	if trimmedPath == "" {
		return "/"
	}
	return trimmedPath
}

func normalizedRoutePrefix(routePrefix string) string {
	if routePrefix == "/" {
		return routePrefix
	}
	return strings.TrimRight(routePrefix, "/")
}

func validationErrorMessage(err error, matched matchedRoute) string {
	if matched.requestPath == "" || matched.requestPath == matched.path {
		return err.Error()
	}
	message := err.Error()
	if !strings.Contains(message, matched.path) {
		return message
	}
	return fmt.Sprintf("%s (%s upstream)", strings.Replace(message, matched.path, matched.requestPath, 1), matched.path)
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

	if svc.Login.Path != "" || svc.PlaceholderAuth == "" {
		return nil
	}

	return validatePlaceholderCredentials(req, svc)
}

func validatePlaceholderCredentials(req *http.Request, svc config.Service) error {
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
	if rawTarget == "" {
		return "", nil
	}
	header, err := parseHeaderAuthTarget(rawTarget)
	if err != nil {
		return "", err
	}

	return req.Header.Get(header), nil
}

func parseHeaderAuthTarget(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("auth target cannot be empty")
	}
	if !strings.Contains(raw, ":") {
		return raw, nil
	}

	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("invalid auth target %q", raw)
	}
	if !strings.EqualFold(parts[0], "header") {
		return "", fmt.Errorf("unsupported auth target %q", raw)
	}
	return parts[1], nil
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
