package signal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/signal"
	"github.com/vinaysrao1/nest/internal/store"
)

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := signal.NewRegistry()
	a := signal.NewTextRegexAdapter()
	r.Register(a)

	got := r.Get("text-regex")
	if got == nil {
		t.Fatal("expected to get registered adapter, got nil")
	}
	if got.ID() != "text-regex" {
		t.Fatalf("unexpected ID: %q", got.ID())
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	r := signal.NewRegistry()
	got := r.Get("no-such-adapter")
	if got != nil {
		t.Fatalf("expected nil for unknown adapter, got %v", got)
	}
}

func TestRegistry_OverwriteSameID(t *testing.T) {
	r := signal.NewRegistry()

	first := signal.NewTextRegexAdapter()
	r.Register(first)

	// Register a second adapter under the same ID to simulate overwrite.
	second := &stubAdapter{id: "text-regex", displayName: "overwritten"}
	r.Register(second)

	got := r.Get("text-regex")
	if got == nil {
		t.Fatal("expected adapter, got nil")
	}
	if got.DisplayName() != "overwritten" {
		t.Fatalf("expected overwritten adapter, got DisplayName=%q", got.DisplayName())
	}
}

func TestRegistry_AllReturnsAll(t *testing.T) {
	r := signal.NewRegistry()
	r.Register(signal.NewTextRegexAdapter())
	r.Register(&stubAdapter{id: "stub-1"})
	r.Register(&stubAdapter{id: "stub-2"})

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 adapters, got %d", len(all))
	}
}

func TestRegistry_AllNonNilWhenEmpty(t *testing.T) {
	r := signal.NewRegistry()
	all := r.All()
	if all == nil {
		t.Fatal("All() must return non-nil slice")
	}
	if len(all) != 0 {
		t.Fatalf("expected empty slice, got len=%d", len(all))
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := signal.NewRegistry()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Mix of Register, Get, All calls concurrently.
	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("adapter-%d", n%5)
			r.Register(&stubAdapter{id: id})
			_ = r.Get(id)
			_ = r.All()
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// TextRegexAdapter tests
// ---------------------------------------------------------------------------

func TestTextRegexAdapter_InterfaceMetadata(t *testing.T) {
	a := signal.NewTextRegexAdapter()

	if a.ID() != "text-regex" {
		t.Errorf("unexpected ID: %q", a.ID())
	}
	if a.DisplayName() == "" {
		t.Error("DisplayName must not be empty")
	}
	if a.Description() == "" {
		t.Error("Description must not be empty")
	}
	if a.Cost() != 1 {
		t.Errorf("unexpected Cost: %d", a.Cost())
	}
	inputs := a.EligibleInputs()
	if len(inputs) == 0 {
		t.Error("EligibleInputs must not be empty")
	}
	if inputs[0] != "text" {
		t.Errorf("unexpected EligibleInputs[0]: %q", inputs[0])
	}
}

func TestTextRegexAdapter_Run(t *testing.T) {
	a := signal.NewTextRegexAdapter()
	ctx := context.Background()

	tests := []struct {
		name      string
		value     string
		wantScore float64
		wantErr   bool
	}{
		{
			name:      "matching pattern",
			value:     "foo\nthis is a foo bar",
			wantScore: 1.0,
		},
		{
			name:      "non-matching pattern",
			value:     "foo\nthis has no match",
			wantScore: 0.0,
		},
		{
			name:    "missing separator",
			value:   "patternWithoutNewline",
			wantErr: true,
		},
		{
			name:    "invalid regex pattern",
			value:   "[invalid\nsome text",
			wantErr: true,
		},
		{
			name:      "case sensitive by default - no match",
			value:     "FOO\nthis is a foo bar",
			wantScore: 0.0,
		},
		{
			name:      "case insensitive with (?i) flag",
			value:     "(?i)FOO\nthis is a foo bar",
			wantScore: 1.0,
		},
		{
			name:      "empty text matches empty pattern",
			value:     "\n",
			wantScore: 1.0,
		},
		{
			name:      "pattern with anchors - full match",
			value:     "^hello$\nhello",
			wantScore: 1.0,
		},
		{
			name:      "pattern with anchors - no match on partial",
			value:     "^hello$\nhello world",
			wantScore: 0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := a.Run(ctx, domain.SignalInput{Type: "text", Value: tc.value})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Score != tc.wantScore {
				t.Errorf("expected score %v, got %v", tc.wantScore, out.Score)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TextBankAdapter tests - mock infrastructure
// ---------------------------------------------------------------------------

// mockDBTX implements store.DBTX for testing.
// Only Query is used by GetTextBankEntries; other methods return errors.
type mockDBTX struct {
	queryFn func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (m *mockDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("mockDBTX: Exec not implemented")
}

func (m *mockDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return m.queryFn(ctx, sql, args...)
}

func (m *mockDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &errRow{err: fmt.Errorf("mockDBTX: QueryRow not implemented")}
}

func (m *mockDBTX) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, fmt.Errorf("mockDBTX: CopyFrom not implemented")
}

// errRow is a pgx.Row that always returns an error on Scan.
type errRow struct{ err error }

func (r *errRow) Scan(dest ...any) error          { return r.err }
func (r *errRow) ScanRow(rows pgx.Rows) error     { return r.err }

// mockRows implements pgx.Rows for a pre-configured slice of TextBankEntry.
type mockRows struct {
	entries []domain.TextBankEntry
	idx     int
	err     error
}

func newMockRows(entries []domain.TextBankEntry) *mockRows {
	return &mockRows{entries: entries, idx: -1}
}

func (r *mockRows) Next() bool {
	if r.idx+1 >= len(r.entries) {
		return false
	}
	r.idx++
	return true
}

func (r *mockRows) Scan(dest ...any) error {
	if r.idx < 0 || r.idx >= len(r.entries) {
		return fmt.Errorf("mockRows: Scan called out of bounds")
	}
	if len(dest) != 5 {
		return fmt.Errorf("mockRows: expected 5 dest args, got %d", len(dest))
	}
	e := r.entries[r.idx]
	*dest[0].(*string) = e.ID
	*dest[1].(*string) = e.TextBankID
	*dest[2].(*string) = e.Value
	*dest[3].(*bool) = e.IsRegex
	*dest[4].(*time.Time) = e.CreatedAt
	return nil
}

func (r *mockRows) Close()                                       {}
func (r *mockRows) Err() error                                   { return r.err }
func (r *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Conn() *pgx.Conn                              { return nil }

// buildTextBankStore creates a *store.Queries backed by a mockDBTX that returns
// the provided entries for any Query call.
func buildTextBankStore(entries []domain.TextBankEntry) *store.Queries {
	dbtx := &mockDBTX{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return newMockRows(entries), nil
		},
	}
	return store.NewWithDBTX(dbtx)
}

// buildErrorStore creates a *store.Queries that returns an error from Query.
func buildErrorStore(queryErr error) *store.Queries {
	dbtx := &mockDBTX{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, queryErr
		},
	}
	return store.NewWithDBTX(dbtx)
}

// ---------------------------------------------------------------------------
// TextBankAdapter tests
// ---------------------------------------------------------------------------

func TestTextBankAdapter_InterfaceMetadata(t *testing.T) {
	a := signal.NewTextBankAdapter(buildTextBankStore(nil))

	if a.ID() != "text-bank" {
		t.Errorf("unexpected ID: %q", a.ID())
	}
	if a.DisplayName() == "" {
		t.Error("DisplayName must not be empty")
	}
	if a.Description() == "" {
		t.Error("Description must not be empty")
	}
	if a.Cost() != 2 {
		t.Errorf("unexpected Cost: %d", a.Cost())
	}
	inputs := a.EligibleInputs()
	if len(inputs) == 0 || inputs[0] != "text_bank" {
		t.Errorf("unexpected EligibleInputs: %v", inputs)
	}
}

func TestTextBankAdapter_Run(t *testing.T) {
	now := time.Now()

	exactEntry := domain.TextBankEntry{
		ID: "e1", TextBankID: "bank1", Value: "badword", IsRegex: false, CreatedAt: now,
	}
	regexEntry := domain.TextBankEntry{
		ID: "e2", TextBankID: "bank1", Value: "bad.*word", IsRegex: true, CreatedAt: now,
	}
	invalidRegexEntry := domain.TextBankEntry{
		ID: "e3", TextBankID: "bank1", Value: "[invalid", IsRegex: true, CreatedAt: now,
	}

	tests := []struct {
		name      string
		entries   []domain.TextBankEntry
		orgID     string
		inputVal  string
		wantScore float64
		wantLabel string
		wantErr   bool
	}{
		{
			name:      "exact match",
			entries:   []domain.TextBankEntry{exactEntry},
			orgID:     "org1",
			inputVal:  "bank1\nthis has badword in it",
			wantScore: 1.0,
			wantLabel: "badword",
		},
		{
			name:      "no match",
			entries:   []domain.TextBankEntry{exactEntry},
			orgID:     "org1",
			inputVal:  "bank1\nclean text here",
			wantScore: 0.0,
		},
		{
			name:      "regex match",
			entries:   []domain.TextBankEntry{regexEntry},
			orgID:     "org1",
			inputVal:  "bank1\nbad1word",
			wantScore: 1.0,
			wantLabel: "bad.*word",
		},
		{
			name:      "invalid regex entry is skipped",
			entries:   []domain.TextBankEntry{invalidRegexEntry, exactEntry},
			orgID:     "org1",
			inputVal:  "bank1\nthis has badword",
			wantScore: 1.0,
			wantLabel: "badword",
		},
		{
			name:     "missing orgID returns error",
			entries:  []domain.TextBankEntry{exactEntry},
			orgID:    "", // no orgID in context
			inputVal: "bank1\nsome text",
			wantErr:  true,
		},
		{
			name:     "missing separator returns error",
			entries:  []domain.TextBankEntry{exactEntry},
			orgID:    "org1",
			inputVal: "bank1-no-newline",
			wantErr:  true,
		},
		{
			name:      "empty bank returns zero score",
			entries:   []domain.TextBankEntry{},
			orgID:     "org1",
			inputVal:  "bank1\nsome text",
			wantScore: 0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := buildTextBankStore(tc.entries)
			a := signal.NewTextBankAdapter(st)

			ctx := context.Background()
			if tc.orgID != "" {
				ctx = signal.WithOrgID(ctx, tc.orgID)
			}

			out, err := a.Run(ctx, domain.SignalInput{Type: "text_bank", Value: tc.inputVal})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Score != tc.wantScore {
				t.Errorf("expected score %v, got %v", tc.wantScore, out.Score)
			}
			if tc.wantLabel != "" && out.Label != tc.wantLabel {
				t.Errorf("expected label %q, got %q", tc.wantLabel, out.Label)
			}
		})
	}
}

func TestTextBankAdapter_DBError(t *testing.T) {
	st := buildErrorStore(fmt.Errorf("db connection failure"))
	a := signal.NewTextBankAdapter(st)

	ctx := signal.WithOrgID(context.Background(), "org1")
	_, err := a.Run(ctx, domain.SignalInput{Type: "text_bank", Value: "bank1\nsome text"})
	if err == nil {
		t.Fatal("expected error from DB failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// Context helpers tests
// ---------------------------------------------------------------------------

func TestWithOrgID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	ctx = signal.WithOrgID(ctx, "org-abc")
	got := signal.OrgIDFromContext(ctx)
	if got != "org-abc" {
		t.Errorf("expected org-abc, got %q", got)
	}
}

func TestOrgIDFromContext_MissingReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	got := signal.OrgIDFromContext(ctx)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// HTTPSignalAdapter tests
// ---------------------------------------------------------------------------

func TestHTTPSignalAdapter_InterfaceMetadata(t *testing.T) {
	a := signal.NewHTTPSignalAdapter("my-http", "My HTTP", "Does stuff", "http://example.com", nil, 0)

	if a.ID() != "my-http" {
		t.Errorf("unexpected ID: %q", a.ID())
	}
	if a.DisplayName() != "My HTTP" {
		t.Errorf("unexpected DisplayName: %q", a.DisplayName())
	}
	if a.Description() != "Does stuff" {
		t.Errorf("unexpected Description: %q", a.Description())
	}
	if a.Cost() != 10 {
		t.Errorf("unexpected Cost: %d", a.Cost())
	}
	inputs := a.EligibleInputs()
	if len(inputs) != 2 {
		t.Errorf("expected 2 eligible inputs, got %d", len(inputs))
	}
}

func TestHTTPSignalAdapter_SuccessfulResponse(t *testing.T) {
	want := domain.SignalOutput{
		Score: 0.85,
		Label: "spam",
		Metadata: map[string]any{
			"confidence": float64(0.9),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"score":    want.Score,
			"label":    want.Label,
			"metadata": want.Metadata,
		})
	}))
	defer srv.Close()

	a := signal.NewHTTPSignalAdapter("test", "Test", "desc", srv.URL, nil, 5*time.Second)
	out, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Score != want.Score {
		t.Errorf("expected score %v, got %v", want.Score, out.Score)
	}
	if out.Label != want.Label {
		t.Errorf("expected label %q, got %q", want.Label, out.Label)
	}
}

func TestHTTPSignalAdapter_Non200Response(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := signal.NewHTTPSignalAdapter("test", "Test", "desc", srv.URL, nil, 5*time.Second)
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestHTTPSignalAdapter_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json{{{"))
	}))
	defer srv.Close()

	a := signal.NewHTTPSignalAdapter("test", "Test", "desc", srv.URL, nil, 5*time.Second)
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHTTPSignalAdapter_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Adapter with a very short timeout.
	a := signal.NewHTTPSignalAdapter("test", "Test", "desc", srv.URL, nil, 50*time.Millisecond)
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestHTTPSignalAdapter_CorrectRequestBody(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf []byte
		buf = make([]byte, r.ContentLength)
		r.Body.Read(buf)
		capturedBody = buf
		json.NewEncoder(w).Encode(map[string]any{"score": 0.0, "label": ""})
	}))
	defer srv.Close()

	a := signal.NewHTTPSignalAdapter("test", "Test", "desc", srv.URL, nil, 5*time.Second)
	a.Run(context.Background(), domain.SignalInput{Type: "image_url", Value: "http://img.example.com/photo.jpg"})

	var decoded map[string]any
	if err := json.Unmarshal(capturedBody, &decoded); err != nil {
		t.Fatalf("captured body is not valid JSON: %v", err)
	}
	if decoded["type"] != "image_url" {
		t.Errorf("expected type=image_url, got %v", decoded["type"])
	}
	if decoded["value"] != "http://img.example.com/photo.jpg" {
		t.Errorf("unexpected value: %v", decoded["value"])
	}
}

func TestHTTPSignalAdapter_CustomHeadersSent(t *testing.T) {
	var capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"score": 0.5, "label": ""})
	}))
	defer srv.Close()

	headers := map[string]string{"Authorization": "Bearer secret-token"}
	a := signal.NewHTTPSignalAdapter("test", "Test", "desc", srv.URL, headers, 5*time.Second)
	a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})

	if capturedAuth != "Bearer secret-token" {
		t.Errorf("expected Authorization header, got %q", capturedAuth)
	}
}

func TestHTTPSignalAdapter_DefaultTimeout(t *testing.T) {
	// Passing timeout=0 should not panic and should use the default 5s.
	a := signal.NewHTTPSignalAdapter("test", "Test", "desc", "http://localhost:9", nil, 0)
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
}

// ---------------------------------------------------------------------------
// stubAdapter — minimal Adapter used by registry tests
// ---------------------------------------------------------------------------

type stubAdapter struct {
	id          string
	displayName string
}

func (s *stubAdapter) ID() string                             { return s.id }
func (s *stubAdapter) DisplayName() string                    { return s.displayName }
func (s *stubAdapter) Description() string                    { return "stub" }
func (s *stubAdapter) EligibleInputs() []domain.SignalInputType { return nil }
func (s *stubAdapter) Cost() int                              { return 0 }
func (s *stubAdapter) Run(_ context.Context, _ domain.SignalInput) (domain.SignalOutput, error) {
	return domain.SignalOutput{}, nil
}
