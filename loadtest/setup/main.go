// Package main implements the Nest test setup program.
//
// It creates item types, an action, rules (from Starlark files on disk), and
// an API key via the Nest REST API. All created IDs and the raw API key are
// written to well-known temp files for downstream tools (generator, validator).
//
// Usage:
//
//	./setup \
//	    -nest-url   http://localhost:8080 \
//	    -rules-dir  ./rules \
//	    -webhook-url http://localhost:9090/webhook
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
)

// apiKeyFile is the path where the raw API key is written.
const apiKeyFile = "/tmp/nest_test_api_key.txt"

// itemTypesFile is the path where item type IDs are written.
const itemTypesFile = "/tmp/nest_test_item_types.txt"

// adminEmail is the seed admin email created by cmd/seed.
const adminEmail = "admin@nest.local"

// adminPassword is the seed admin password created by cmd/seed.
const adminPassword = "admin123"

func main() {
	nestURL := flag.String("nest-url", "http://localhost:8080", "Nest API base URL")
	rulesDir := flag.String("rules-dir", "./rules", "Directory containing Starlark rule files")
	webhookURL := flag.String("webhook-url", "http://localhost:9090/webhook", "Webhook receiver URL for actions")
	flag.Parse()

	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("[setup] cookiejar: %v", err)
	}
	client := &http.Client{Jar: jar}

	// Step 1: Login.
	session, csrf, err := login(client, *nestURL, adminEmail, adminPassword)
	if err != nil {
		log.Fatalf("[setup] login failed: %v", err)
	}
	_ = session // session cookie is managed by the jar automatically

	// Step 1b: Rotate signing key so webhook actions can be signed.
	if err := rotateSigningKey(client, *nestURL, session, csrf); err != nil {
		log.Fatalf("[setup] rotate signing key: %v", err)
	}
	fmt.Println("[setup] Rotated signing key")

	// Step 2: Create item types.
	postID, err := createItemType(client, *nestURL, session, csrf, "post", "CONTENT", map[string]any{
		"text":      "string",
		"entity_id": "string",
	})
	if err != nil {
		log.Fatalf("[setup] create item type 'post': %v", err)
	}
	fmt.Printf("[setup] Item type: post (%s)\n", postID)

	likeID, err := createItemType(client, *nestURL, session, csrf, "like", "CONTENT", map[string]any{
		"entity_id":   "string",
		"subject_uri": "string",
	})
	if err != nil {
		log.Fatalf("[setup] create item type 'like': %v", err)
	}
	fmt.Printf("[setup] Item type: like (%s)\n", likeID)

	// Step 3: Create webhook action.
	actionID, err := createAction(client, *nestURL, session, csrf, "webhook-notify", "WEBHOOK", map[string]any{
		"url": *webhookURL,
	})
	if err != nil {
		log.Fatalf("[setup] create action 'webhook-notify': %v", err)
	}
	fmt.Printf("[setup] Action: webhook-notify (%s)\n", actionID)

	// Step 3b: Create MRT enqueue action.
	mrtActionID, err := createAction(client, *nestURL, session, csrf, "mrt-review", "ENQUEUE_TO_MRT", map[string]any{
		"queue_name": "default",
	})
	if err != nil {
		log.Fatalf("[setup] create action 'mrt-review': %v", err)
	}
	fmt.Printf("[setup] Action: mrt-review (%s)\n", mrtActionID)

	// Step 3c: Create MRT queue for OpenAI moderation (only when OpenAI is enabled).
	// The openai-moderation rule is only loaded when OPENAI_API_KEY is set, so the
	// queue is only needed in that case.
	if os.Getenv("OPENAI_API_KEY") != "" {
		fmt.Println("[setup] Creating MRT queue: openai")
		openaiQueueID, err := createMRTQueue(client, *nestURL, session, csrf, "openai", "OpenAI moderation review queue")
		if err != nil {
			log.Fatalf("[setup] create MRT queue 'openai': %v", err)
		}
		fmt.Printf("[setup] MRT Queue: openai (%s)\n", openaiQueueID)
	}

	// Step 4: Create rules from Starlark files.
	if err := createRulesFromDir(client, *nestURL, session, csrf, *rulesDir); err != nil {
		log.Fatalf("[setup] create rules: %v", err)
	}

	// Load optional rules that require external API keys.
	// The optional/ subdirectory is only loaded when the relevant env var is set,
	// so missing signal adapters never cause spurious errors at runtime.
	if os.Getenv("OPENAI_API_KEY") != "" {
		optionalDir := filepath.Join(*rulesDir, "optional")
		fmt.Println("[setup] OPENAI_API_KEY set — loading optional rules from", optionalDir)
		if err := createRulesFromDir(client, *nestURL, session, csrf, optionalDir); err != nil {
			log.Fatalf("[setup] create optional rules: %v", err)
		}
	} else {
		fmt.Println("[setup] OPENAI_API_KEY not set — skipping optional rules")
	}

	// Step 5: Create API key and persist it.
	rawKey, err := createAPIKey(client, *nestURL, session, csrf, "nest-test-key")
	if err != nil {
		log.Fatalf("[setup] create API key: %v", err)
	}
	if err := os.WriteFile(apiKeyFile, []byte(rawKey), 0600); err != nil {
		log.Fatalf("[setup] write API key file: %v", err)
	}
	fmt.Printf("[setup] Created API key: nest-test-key\n")

	// Step 6: Persist item type IDs.
	itemTypesContent := fmt.Sprintf("post=%s\nlike=%s\n", postID, likeID)
	if err := os.WriteFile(itemTypesFile, []byte(itemTypesContent), 0644); err != nil {
		log.Fatalf("[setup] write item types file: %v", err)
	}

	fmt.Println("[setup] Setup complete.")
}

// login authenticates with email/password and returns the session cookie value
// and CSRF token. The CookieJar on client automatically receives the session
// cookie, but the caller may use the returned session string if needed.
//
// POST /api/v1/auth/login
func login(client *http.Client, baseURL, email, password string) (sessionCookie string, csrfToken string, err error) {
	body := map[string]string{
		"email":    email,
		"password": password,
	}
	resp, respBody, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/auth/login", "", "", body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", fmt.Errorf("parse login response: %w", err)
	}
	if parsed.CSRFToken == "" {
		return "", "", fmt.Errorf("login response missing csrf_token")
	}

	// Extract session cookie value for callers that need it explicitly.
	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			sessionCookie = c.Value
			break
		}
	}

	return sessionCookie, parsed.CSRFToken, nil
}

// createItemType creates an item type and returns its ID.
// If the item type already exists (409), it fetches the existing one by name.
//
// POST /api/v1/item-types
func createItemType(client *http.Client, baseURL, session, csrf, name, kind string, schema map[string]any) (id string, err error) {
	body := map[string]any{
		"name":   name,
		"kind":   kind,
		"schema": schema,
	}
	resp, respBody, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/item-types", session, csrf, body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusConflict {
		return findResourceByName(client, baseURL+"/api/v1/item-types", session, csrf, name)
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse item type response: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("item type response missing id")
	}
	return parsed.ID, nil
}

// createAction creates an action and returns its ID.
// If the action already exists (409), it fetches the existing one by name.
//
// POST /api/v1/actions
func createAction(client *http.Client, baseURL, session, csrf, name, actionType string, config map[string]any) (id string, err error) {
	body := map[string]any{
		"name":        name,
		"action_type": actionType,
		"config":      config,
	}
	resp, respBody, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/actions", session, csrf, body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusConflict {
		return findResourceByName(client, baseURL+"/api/v1/actions", session, csrf, name)
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse action response: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("action response missing id")
	}
	return parsed.ID, nil
}

// createMRTQueue creates an MRT queue and returns its ID.
// If the queue already exists (409), it fetches the existing one by name.
//
// POST /api/v1/mrt/queues
func createMRTQueue(client *http.Client, baseURL, session, csrf, name, description string) (string, error) {
	body := map[string]any{
		"name":        name,
		"description": description,
	}
	resp, respBody, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/mrt/queues", session, csrf, body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusConflict {
		fmt.Printf("[setup] MRT queue %q already exists, fetching existing\n", name)
		return findResourceByName(client, baseURL+"/api/v1/mrt/queues", session, csrf, name)
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse mrt queue response: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("mrt queue response missing id")
	}
	return parsed.ID, nil
}

// createRule creates a rule from source and returns its ID.
// If the rule already exists (409), it fetches the existing one by name.
//
// POST /api/v1/rules
func createRule(client *http.Client, baseURL, session, csrf, name, source string) (id string, err error) {
	body := map[string]any{
		"name":   name,
		"status": "LIVE",
		"source": source,
	}
	resp, respBody, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/rules", session, csrf, body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusConflict {
		return findResourceByName(client, baseURL+"/api/v1/rules", session, csrf, name)
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse rule response: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("rule response missing id")
	}
	return parsed.ID, nil
}

// createAPIKey creates a named API key and returns the plaintext raw key.
// If an API key with that name already exists (409), it reads the key from
// the previously persisted file rather than failing.
//
// POST /api/v1/api-keys
func createAPIKey(client *http.Client, baseURL, session, csrf, name string) (rawKey string, err error) {
	body := map[string]string{"name": name}
	resp, respBody, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/api-keys", session, csrf, body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusConflict {
		// Key already exists; try to read the previously saved key file.
		existing, readErr := os.ReadFile(apiKeyFile)
		if readErr != nil {
			return "", fmt.Errorf("API key already exists and cannot read saved key: %w", readErr)
		}
		return strings.TrimSpace(string(existing)), nil
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse api-key response: %w", err)
	}
	if parsed.Key == "" {
		return "", fmt.Errorf("api-key response missing key")
	}
	return parsed.Key, nil
}

// createRulesFromDir reads every *.star file in dir and creates a rule for each.
// The rule name is derived from the filename without extension.
func createRulesFromDir(client *http.Client, baseURL, session, csrf, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read rules dir %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".star") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read rule file %q: %w", path, err)
		}

		name := strings.TrimSuffix(entry.Name(), ".star")
		// Replace underscores with hyphens to match Starlark rule_id convention.
		name = strings.ReplaceAll(name, "_", "-")

		id, err := createRule(client, baseURL, session, csrf, name, string(source))
		if err != nil {
			return fmt.Errorf("create rule %q: %w", name, err)
		}
		fmt.Printf("[setup] Created rule: %s (%s)\n", name, id)
	}

	return nil
}

// rotateSigningKey generates a new RSA signing key for the authenticated org by
// calling POST /api/v1/signing-keys/rotate. A signing key must exist before any
// webhook actions can be delivered.
//
// POST /api/v1/signing-keys/rotate
func rotateSigningKey(client *http.Client, baseURL, session, csrf string) error {
	resp, respBody, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/signing-keys/rotate", session, csrf, map[string]any{})
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// findResourceByName fetches the list endpoint and returns the ID of the
// resource matching the given name. Used as a fallback when create returns 409.
func findResourceByName(client *http.Client, listURL, session, csrf, name string) (string, error) {
	resp, respBody, err := doJSON(client, http.MethodGet, listURL, session, csrf, nil)
	if err != nil {
		return "", fmt.Errorf("list resources: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list resources: unexpected status %d: %s", resp.StatusCode, respBody)
	}

	// Response may be {"items": [...]} or a bare array.
	var items []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var wrapper struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(respBody, &wrapper); err == nil && len(wrapper.Items) > 0 {
		items = wrapper.Items
	} else {
		_ = json.Unmarshal(respBody, &items)
	}

	for _, item := range items {
		if item.Name == name {
			return item.ID, nil
		}
	}
	return "", fmt.Errorf("resource %q not found in list response", name)
}

// doJSON encodes body as JSON, POSTs (or uses method) to url, and returns the
// response and its body bytes. It sets the session cookie and X-Csrf-Token
// header when session/csrf are non-empty.
func doJSON(client *http.Client, method, url, session, csrf string, body any) (*http.Response, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if session != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: session})
	}
	if csrf != "" {
		req.Header.Set("X-Csrf-Token", csrf)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response body: %w", err)
	}

	return resp, respBody, nil
}
