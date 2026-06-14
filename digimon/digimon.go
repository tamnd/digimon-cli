// Package digimon is the library behind the digimon command line:
// the HTTP client, request shaping, and the typed data models for the
// Digimon API at digi-api.com.
//
// The Client sets a real User-Agent, paces requests so a busy session stays
// polite, and retries transient failures (429 and 5xx). Build your endpoint
// calls and JSON decoding on top of it.
package digimon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DefaultUserAgent identifies the client to digi-api.com.
const DefaultUserAgent = "digimon/dev (+https://github.com/tamnd/digimon-cli)"

// Host is the site this client talks to.
const Host = "digi-api.com"

// baseURL is the root every API request is built from.
const baseURL = "https://digi-api.com"

// Client talks to the Digimon API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   3,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client settings. The body is read fully and closed here.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types (unexported, internal JSON shapes from digi-api.com) ---

type wireDigimon struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	XAntibody bool   `json:"xAntibody"`
	Images    []struct {
		Href string `json:"href"`
	} `json:"images"`
	Levels []struct {
		Level string `json:"level"`
	} `json:"levels"`
	Types []struct {
		Type string `json:"type"`
	} `json:"types"`
	Attributes []struct {
		Attribute string `json:"attribute"`
	} `json:"attributes"`
	ReleaseDate  string `json:"releaseDate"`
	Descriptions []struct {
		Origin      string `json:"origin"`
		Language    string `json:"language"`
		Description string `json:"description"`
	} `json:"descriptions"`
	Skills []struct {
		Skill string `json:"skill"`
	} `json:"skills"`
	NextEvolutions []struct {
		Digimon string `json:"digimon"`
	} `json:"nextEvolutions"`
	PriorEvolutions []struct {
		Digimon string `json:"digimon"`
	} `json:"priorEvolutions"`
}

type wireStub struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

type wireList struct {
	Content  []wireStub `json:"content"`
	Pageable struct {
		TotalElements int `json:"totalElements"`
	} `json:"pageable"`
}

// --- output types ---

// Digimon is the full record returned by the digimon command.
type Digimon struct {
	ID          int      `kit:"id" json:"id"`
	Name        string   `json:"name"`
	Level       string   `json:"level"`
	Type        string   `json:"type"`
	Attribute   string   `json:"attribute"`
	Description string   `json:"description" kit:"body"`
	ReleaseDate string   `json:"release_date"`
	Image       string   `json:"image"`
	Skills      []string `json:"skills"`
	NextEvos    []string `json:"next_evolutions"`
	PriorEvos   []string `json:"prior_evolutions"`
}

// DigimonStub is the lightweight record returned by list and search.
type DigimonStub struct {
	ID    int    `kit:"id" json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

// --- client methods ---

// ListDigimon returns up to limit Digimon stubs from the first page.
func (c *Client) ListDigimon(ctx context.Context, limit int) ([]DigimonStub, error) {
	u := fmt.Sprintf("%s/api/v1/digimon?page=0&pageSize=%d", baseURL, limit)
	return c.fetchStubs(ctx, u)
}

// SearchDigimon returns up to limit Digimon stubs matching name.
func (c *Client) SearchDigimon(ctx context.Context, name string, limit int) ([]DigimonStub, error) {
	u := fmt.Sprintf("%s/api/v1/digimon?name=%s&pageSize=%d", baseURL, url.QueryEscape(name), limit)
	return c.fetchStubs(ctx, u)
}

// GetDigimon fetches full details for a single Digimon by name or numeric ID.
func (c *Client) GetDigimon(ctx context.Context, nameOrID string) (*Digimon, error) {
	u := fmt.Sprintf("%s/api/v1/digimon/%s", baseURL, url.PathEscape(nameOrID))
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var w wireDigimon
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("decode digimon: %w", err)
	}
	return convertDigimon(w), nil
}

func (c *Client) fetchStubs(ctx context.Context, u string) ([]DigimonStub, error) {
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wl wireList
	if err := json.Unmarshal(body, &wl); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	out := make([]DigimonStub, 0, len(wl.Content))
	for _, s := range wl.Content {
		out = append(out, DigimonStub(s))
	}
	return out, nil
}

// --- conversion ---

func convertDigimon(w wireDigimon) *Digimon {
	d := &Digimon{
		ID:          w.ID,
		Name:        w.Name,
		Description: extractDesc(w.Descriptions),
		ReleaseDate: w.ReleaseDate,
	}
	if len(w.Images) > 0 {
		d.Image = w.Images[0].Href
	}
	if len(w.Levels) > 0 {
		d.Level = w.Levels[0].Level
	}
	if len(w.Types) > 0 {
		d.Type = w.Types[0].Type
	}
	if len(w.Attributes) > 0 {
		d.Attribute = w.Attributes[0].Attribute
	}
	for _, s := range w.Skills {
		d.Skills = append(d.Skills, s.Skill)
	}
	for _, e := range w.NextEvolutions {
		d.NextEvos = append(d.NextEvos, e.Digimon)
	}
	for _, e := range w.PriorEvolutions {
		d.PriorEvos = append(d.PriorEvos, e.Digimon)
	}
	return d
}

func extractDesc(descs []struct {
	Origin      string `json:"origin"`
	Language    string `json:"language"`
	Description string `json:"description"`
}) string {
	for _, d := range descs {
		if d.Language == "en_us" && d.Origin == "Reference Book" {
			return d.Description
		}
	}
	for _, d := range descs {
		if d.Language == "en_us" {
			return d.Description
		}
	}
	return ""
}
