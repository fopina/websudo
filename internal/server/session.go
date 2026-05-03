package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/fopina/websudo/internal/config"
)

const encryptedCookiePrefix = "wsenc:"

type routeContext struct {
	serviceName string
	variantName string
	service     config.Service
}

func isLoginRequest(req *http.Request, svc config.Service) bool {
	return svc.Login.Path != "" && strings.EqualFold(req.Method, http.MethodPost) && req.URL.Path == svc.Login.Path
}

func rewriteLoginRequest(req *http.Request, svc config.Service) error {
	if !strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "application/x-www-form-urlencoded") {
		return fmt.Errorf("login request content-type %q is not supported", req.Header.Get("Content-Type"))
	}

	username, password, err := svc.Login.LoginCredentials()
	if err != nil {
		return err
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("read login request body: %w", err)
	}
	_ = req.Body.Close()

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return fmt.Errorf("parse login request body: %w", err)
	}
	values.Set(svc.Login.UsernameField, username)
	values.Set(svc.Login.PasswordField, password)
	encoded := values.Encode()

	req.Body = io.NopCloser(strings.NewReader(encoded))
	req.ContentLength = int64(len(encoded))
	req.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
	req.TransferEncoding = nil
	return nil
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
