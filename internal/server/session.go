package server

import (
	"bytes"
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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const encryptedCookiePrefix = "wsenc:"

var errLoginPlaceholderCredentials = errors.New("login placeholder credentials do not match")

type routeContext struct {
	serviceName string
	variantName string
	service     config.Service
	isLogin     bool
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
	var parsed json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("parse login request JSON body: %w", err)
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return fmt.Errorf("parse login request JSON body: expected object")
	}
	rawUsername := gjson.GetBytes(body, svc.Login.UsernameField)
	rawPassword := gjson.GetBytes(body, svc.Login.PasswordField)
	submittedUsername := rawUsername.String()
	submittedPassword := rawPassword.String()
	if rawUsername.Type != gjson.String {
		submittedUsername = ""
	}
	if rawPassword.Type != gjson.String {
		submittedPassword = ""
	}
	if err := validateLoginPlaceholderCredentials(svc, submittedUsername, submittedPassword); err != nil {
		return err
	}
	encoded, err := sjson.SetBytes(body, svc.Login.UsernameField, username)
	if err != nil {
		return fmt.Errorf("encode login request JSON body: %w", err)
	}
	encoded, err = sjson.SetBytes(encoded, svc.Login.PasswordField, password)
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

func decryptRequestAuthToken(req *http.Request, svc config.Service) error {
	if svc.AuthMode != config.AuthModeHeader || svc.Login.Path == "" {
		return nil
	}
	key, err := svc.CookieCipherKey()
	if err != nil || len(key) == 0 {
		return err
	}
	header, err := loginAuthHeader(svc)
	if err != nil {
		return err
	}
	value := req.Header.Get(header)
	if value == "" {
		return nil
	}
	if decrypted, ok := decryptAuthHeaderValue(key, header, value); ok {
		req.Header.Set(header, decrypted)
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

func encryptLoginResponseAuthToken(resp *http.Response, svc config.Service) error {
	if svc.AuthMode != config.AuthModeHeader || svc.Login.Path == "" || svc.Login.TokenField == "" || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	key, err := svc.CookieCipherKey()
	if err != nil || len(key) == 0 {
		return err
	}
	header, err := loginAuthHeader(svc)
	if err != nil {
		return err
	}
	if resp.Body == nil {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read login response body: %w", err)
	}
	_ = resp.Body.Close()
	if strings.TrimSpace(string(body)) == "" {
		setResponseBody(resp, body)
		return nil
	}

	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		contentType = strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	}
	if contentType != "" && !strings.EqualFold(contentType, "application/json") {
		return fmt.Errorf("login token response content-type %q is not supported", resp.Header.Get("Content-Type"))
	}

	var parsed json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("parse login token response JSON body: %w", err)
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return fmt.Errorf("parse login token response JSON body: expected object")
	}
	rawToken := gjson.GetBytes(body, svc.Login.TokenField)
	if !rawToken.Exists() {
		return fmt.Errorf("login token response is missing token field %q", svc.Login.TokenField)
	}
	if rawToken.Type != gjson.String || rawToken.String() == "" {
		return fmt.Errorf("login token response token field %q must be a non-empty string", svc.Login.TokenField)
	}
	encrypted, err := encryptCookieValue(key, header, rawToken.String())
	if err != nil {
		return err
	}
	encoded, err := sjson.SetBytes(body, svc.Login.TokenField, encrypted)
	if err != nil {
		return fmt.Errorf("encode login token response JSON body: %w", err)
	}
	setResponseBody(resp, encoded)
	return nil
}

func decryptAuthHeaderValue(key []byte, header string, value string) (string, bool) {
	if plaintext, ok := decryptCookieValue(key, header, value); ok {
		return plaintext, true
	}
	index := strings.LastIndex(value, " "+encryptedCookiePrefix)
	if index < 0 {
		return "", false
	}
	encrypted := value[index+1:]
	plaintext, ok := decryptCookieValue(key, header, encrypted)
	if !ok {
		return "", false
	}
	return value[:index+1] + plaintext, true
}

func loginAuthHeader(svc config.Service) (string, error) {
	target := svc.InjectAuthTarget
	if target == "" {
		target = svc.PlaceholderAuth
	}
	return parseHeaderAuthTarget(target)
}

func setResponseBody(resp *http.Response, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
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
