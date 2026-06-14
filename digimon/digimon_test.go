package digimon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestListDigimon(t *testing.T) {
	fixture := wireList{
		Content: []wireStub{
			{ID: 1, Name: "Agumon", Image: "https://example.com/agumon.png"},
			{ID: 2, Name: "Airdramon", Image: "https://example.com/airdramon.png"},
		},
	}
	fixture.Pageable.TotalElements = 2

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("page") != "0" || q.Get("pageSize") != "2" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	stubs, err := c.ListDigimon(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(stubs) != 2 {
		t.Fatalf("got %d stubs, want 2", len(stubs))
	}
	if stubs[0].Name != "Agumon" {
		t.Errorf("stubs[0].Name = %q, want Agumon", stubs[0].Name)
	}
	if stubs[1].ID != 2 {
		t.Errorf("stubs[1].ID = %d, want 2", stubs[1].ID)
	}
}

func TestSearchDigimon(t *testing.T) {
	fixture := wireList{
		Content: []wireStub{
			{ID: 1, Name: "Agumon", Image: "https://example.com/agumon.png"},
		},
	}
	fixture.Pageable.TotalElements = 1

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("name") != "agumon" {
			t.Errorf("name param = %q, want agumon", q.Get("name"))
		}
		if q.Get("pageSize") != "10" {
			t.Errorf("pageSize param = %q, want 10", q.Get("pageSize"))
		}
		_ = json.NewEncoder(w).Encode(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	stubs, err := c.SearchDigimon(context.Background(), "agumon", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stubs) != 1 || stubs[0].Name != "Agumon" {
		t.Errorf("unexpected stubs: %+v", stubs)
	}
}

func TestGetDigimon(t *testing.T) {
	fixture := wireDigimon{
		ID:          1,
		Name:        "Agumon",
		Images:      []struct{ Href string `json:"href"` }{{Href: "https://example.com/agumon.png"}},
		Levels:      []struct{ Level string `json:"level"` }{{Level: "Child"}},
		Types:       []struct{ Type string `json:"type"` }{{Type: "Reptile"}},
		Attributes:  []struct{ Attribute string `json:"attribute"` }{{Attribute: "Vaccine"}},
		ReleaseDate: "1997-06-26",
		Descriptions: []struct {
			Origin      string `json:"origin"`
			Language    string `json:"language"`
			Description string `json:"description"`
		}{
			{Origin: "Reference Book", Language: "en_us", Description: "A Vaccine Digimon."},
		},
		Skills: []struct {
			Skill string `json:"skill"`
		}{{Skill: "Pepper Breath"}},
		NextEvolutions: []struct {
			Digimon string `json:"digimon"`
		}{{Digimon: "Greymon"}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/digimon/agumon" {
			t.Errorf("path = %q, want /api/v1/digimon/agumon", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	d, err := c.GetDigimon(context.Background(), "agumon")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != 1 {
		t.Errorf("ID = %d, want 1", d.ID)
	}
	if d.Name != "Agumon" {
		t.Errorf("Name = %q, want Agumon", d.Name)
	}
	if d.Level != "Child" {
		t.Errorf("Level = %q, want Child", d.Level)
	}
	if d.Type != "Reptile" {
		t.Errorf("Type = %q, want Reptile", d.Type)
	}
	if d.Attribute != "Vaccine" {
		t.Errorf("Attribute = %q, want Vaccine", d.Attribute)
	}
	if d.Description != "A Vaccine Digimon." {
		t.Errorf("Description = %q, want 'A Vaccine Digimon.'", d.Description)
	}
	if d.ReleaseDate != "1997-06-26" {
		t.Errorf("ReleaseDate = %q, want 1997-06-26", d.ReleaseDate)
	}
	if d.Image != "https://example.com/agumon.png" {
		t.Errorf("Image = %q, unexpected", d.Image)
	}
	if len(d.Skills) != 1 || d.Skills[0] != "Pepper Breath" {
		t.Errorf("Skills = %v, want [Pepper Breath]", d.Skills)
	}
	if len(d.NextEvos) != 1 || d.NextEvos[0] != "Greymon" {
		t.Errorf("NextEvos = %v, want [Greymon]", d.NextEvos)
	}
}

// newTestClient returns a Client that talks to srv instead of digi-api.com,
// by overriding the baseURL constant via a thin wrapper on Get.
func newTestClient(srvURL string) *testClient {
	return &testClient{Client: NewClient(), base: srvURL}
}

// testClient wraps Client and redirects requests to a test server URL.
type testClient struct {
	*Client
	base string
}

func (tc *testClient) ListDigimon(ctx context.Context, limit int) ([]DigimonStub, error) {
	u := tc.base + "/api/v1/digimon?page=0&pageSize=" + itoa(limit)
	return tc.Client.fetchStubs(ctx, u)
}

func (tc *testClient) SearchDigimon(ctx context.Context, name string, limit int) ([]DigimonStub, error) {
	u := tc.base + "/api/v1/digimon?name=" + name + "&pageSize=" + itoa(limit)
	return tc.Client.fetchStubs(ctx, u)
}

func (tc *testClient) GetDigimon(ctx context.Context, nameOrID string) (*Digimon, error) {
	u := tc.base + "/api/v1/digimon/" + nameOrID
	body, err := tc.Client.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var w wireDigimon
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, err
	}
	return convertDigimon(w), nil
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
