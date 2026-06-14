package digimon

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network. The client's HTTP behaviour is
// covered in digimon_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "digimon" {
		t.Errorf("Scheme = %q, want digimon", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "digimon" {
		t.Errorf("Identity.Binary = %q, want digimon", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in, typ, id string
	}{
		{"1", "id", "1"},
		{"42", "id", "42"},
		{"agumon", "name", "agumon"},
		{"Greymon", "name", "greymon"},
		{"https://digi-api.com/api/v1/digimon/1", "id", "1"},
		{"https://digi-api.com/api/v1/digimon/agumon", "name", "agumon"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil {
			t.Errorf("Classify(%q) returned error: %v", tc.in, err)
			continue
		}
		if typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)", tc.in, typ, id, tc.typ, tc.id)
		}
	}
}

func TestClassifyErrors(t *testing.T) {
	bad := []string{"", "://bad", "http://example.com/"}
	for _, in := range bad {
		_, _, err := Domain{}.Classify(in)
		if err == nil {
			t.Errorf("Classify(%q) should have returned an error", in)
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		typ, id, want string
	}{
		{"id", "1", "https://digi-api.com/api/v1/digimon/1"},
		{"name", "agumon", "https://digi-api.com/api/v1/digimon/agumon"},
		{"digimon", "greymon", "https://digi-api.com/api/v1/digimon/greymon"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.typ, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)", tc.typ, tc.id, got, err, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "1")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip:
// a record mints to its URI, its body is readable, and a bare id resolves back.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	d := &Digimon{
		ID:          1,
		Name:        "Agumon",
		Description: "A Vaccine Digimon that breathes fire.",
	}
	u, err := h.Mint(d)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "digimon://digimon/1"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	if body, ok := h.Body(d); !ok || body == "" {
		t.Errorf("Body = (%q, %v), want non-empty", body, ok)
	}

	// "agumon" is a name-like string so Classify maps it to ("name", "agumon").
	got, err := h.ResolveOn("digimon", "agumon")
	if err != nil || got.String() != "digimon://name/agumon" {
		t.Errorf("ResolveOn = (%q, %v), want digimon://name/agumon", got.String(), err)
	}
}
