package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/fopina/websudo/internal/config"
)

const encryptedCookiePrefix = "wsenc:"

var errLoginPlaceholderCredentials = errors.New("login placeholder credentials do not match")

type routeContext struct {
	serviceName string
	variantName string
	service     config.Service
}

func isLoginRequest(req *http.Request, svc config.Service) bool {
	return svc.Login.Path != "" && strings.EqualFold(req.Method, http.MethodPost) && req.URL.Path == svc.Login.Path
}

func rewriteLoginRequest(req *http.Request, svc config.Service) error {
	username, password, err := svc.Login.LoginCredentials()
	if err != nil {
		return err
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("read login request body: %w", err)
	}
	_ = req.Body.Close()

	contentType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		contentType = strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Type")))
	}

	switch strings.ToLower(contentType) {
	case "application/x-www-form-urlencoded":
		return rewriteFormLoginRequest(req, svc, body, username, password)
	case "application/json":
		return rewriteJSONLoginRequest(req, svc, body, username, password)
	default:
		return fmt.Errorf("login request content-type %q is not supported", req.Header.Get("Content-Type"))
	}
}

func rewriteFormLoginRequest(req *http.Request, svc config.Service, body []byte, username string, password string) error {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return fmt.Errorf("parse login request body: %w", err)
	}
	if err := validateLoginPlaceholderCredentials(svc, values.Get(svc.Login.UsernameField), values.Get(svc.Login.PasswordField)); err != nil {
		return err
	}
	values.Set(svc.Login.UsernameField, username)
	values.Set(svc.Login.PasswordField, password)
	encoded := values.Encode()
	setRequestBody(req, encoded)
	return nil
}

func rewriteJSONLoginRequest(req *http.Request, svc config.Service, body []byte, username string, password string) error {
	var values map[string]any
	if err := json.Unmarshal(body, &values); err != nil {
		return fmt.Errorf("parse login request JSON body: %w", err)
	}
	if values == nil {
		return fmt.Errorf("parse login request JSON body: expected object")
	}
	submittedUsername, _ := values[svc.Login.UsernameField].(string)
	submittedPassword, _ := values[svc.Login.PasswordField].(string)
	if err := validateLoginPlaceholderCredentials(svc, submittedUsername, submittedPassword); err != nil {
		return err
	}
	values[svc.Login.UsernameField] = username
	values[svc.Login.PasswordField] = password
	encoded, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode login request JSON body: %w", err)
	}
	setRequestBody(req, string(encoded))
	return nil
}

func validateLoginPlaceholderCredentials(svc config.Service, submittedUsername string, submittedPassword string) error {
	username, password, ok, err := svc.Login.PlaceholderCredentials()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if submittedUsername != username || submittedPassword != password {
		return errLoginPlaceholderCredentials
	}
	return nil
}

func setRequestBody(req *http.Request, body string) {
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	req.TransferEncoding = nil
}

func decryptRequestCookies(req *http.Request, svc config.Service) error {
	key, err := svc.CookieCipherKey()
	if err != nil || len(key) == 0 {
		return err
	}

	cookies := req.Cookies()
	if len(cookies) == 0 {
		return nil
	}

	changed := false
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if plaintext, ok := decryptCookieValue(key, cookie.Name, cookie.Value); ok {
			cookie.Value = plaintext
			changed = true
		}
		parts = append(parts, cookie.String())
	}
	if changed {
		req.Header.Set("Cookie", strings.Join(parts, "; "))
	}
	return nil
}

func encryptResponseCookies(resp *http.Response, svc config.Service) error {
	key, err := svc.CookieCipherKey()
	if err != nil || len(key) == 0 {
		return err
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return nil
	}

	resp.Header.Del("Set-Cookie")
	for _, cookie := range cookies {
		encrypted, err := encryptCookieValue(key, cookie.Name, cookie.Value)
		if err != nil {
			return err
		}
		cookie.Value = encrypted
		resp.Header.Add("Set-Cookie", cookie.String())
	}
	return nil
}

func encryptCookieValue(key []byte, name string, value string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cookie cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create cookie gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate cookie nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(value), []byte(name))
	payload := append(nonce, sealed...)
	return encryptedCookiePrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func decryptCookieValue(key []byte, name string, value string) (string, bool) {
	if !strings.HasPrefix(value, encryptedCookiePrefix) {
		return "", false
	}

	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, encryptedCookiePrefix))
	if err != nil {
		return "", false
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", false
	}
	if len(data) < gcm.NonceSize() {
		return "", false
	}
	plaintext, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte(name))
	if err != nil {
		return "", false
	}
	return string(plaintext), true
}

func responseError(resp *http.Response, req *http.Request, status int, message string) *http.Response {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return goproxy.NewResponse(req, goproxy.ContentTypeText, status, message)
}
