package coze

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (c *Client) getToken(ctx context.Context) (string, error) {
	if token := strings.TrimSpace(c.cfg.CozeAccessToken); token != "" {
		return withBearer(token), nil
	}

	c.mu.Lock()
	if c.cachedToken != "" && time.Now().Add(time.Minute).Before(c.tokenExpiry) {
		token := c.cachedToken
		c.mu.Unlock()
		return withBearer(token), nil
	}
	c.mu.Unlock()

	privateKey, err := c.privateKey()
	if err != nil {
		return "", err
	}
	jwt, err := createOAuthJWT(privateKey, c.cfg.CozeAppID, c.cfg.CozePublicKeyID, c.cfg.CozeBaseURL)
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]any{
		"duration_seconds": 7200,
		"grant_type":       "urn:ietf:params:oauth:grant-type:jwt-bearer",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.CozeBaseURL+"/api/permission/oauth2/token", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode Coze token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || result.AccessToken == "" {
		return "", fmt.Errorf("Coze token request failed with status %d", resp.StatusCode)
	}

	expiry := time.Unix(result.ExpiresIn, 0)
	if result.ExpiresIn <= time.Now().Unix() {
		expiry = time.Now().Add(15 * time.Minute)
	}
	c.mu.Lock()
	c.cachedToken = result.AccessToken
	c.tokenExpiry = expiry
	c.mu.Unlock()
	return withBearer(result.AccessToken), nil
}

func (c *Client) privateKey() (*rsa.PrivateKey, error) {
	value := strings.TrimSpace(c.cfg.CozePrivateKey)
	if value == "" && c.cfg.CozePrivateKeyFile != "" {
		data, err := os.ReadFile(c.cfg.CozePrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read COZE_PRIVATE_KEY_FILE: %w", err)
		}
		value = string(data)
	}
	value = strings.ReplaceAll(value, string([]byte{92, 110}), string([]byte{10}))
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("Coze private key is not valid PEM")
	}
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("Coze private key must be RSA")
		}
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("unable to parse Coze RSA private key")
}

func createOAuthJWT(key *rsa.PrivateKey, clientID, keyID, baseURL string) (string, error) {
	now := time.Now()
	var jtiBytes [24]byte
	if _, err := rand.Read(jtiBytes[:]); err != nil {
		return "", err
	}
	jti := hex.EncodeToString(jtiBytes[:])
	header, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": keyID,
	})
	if err != nil {
		return "", err
	}
	audience := "api.coze.cn"
	if strings.Contains(baseURL, "coze.com") {
		audience = "api.coze.com"
	}
	claims, err := json.Marshal(map[string]any{
		"iss": clientID,
		"aud": audience,
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Hour).Unix(),
		"jti": jti,
	})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func withBearer(token string) string {
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return token
	}
	return "Bearer " + token
}
