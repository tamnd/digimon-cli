package digimon

import (
	"context"
	"net/url"
	"strings"
	"unicode"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes the Digimon API as a kit Domain: a driver that a
// multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/digimon-cli/digimon"
//
// The init below registers it; the host then dereferences digimon:// URIs by
// routing to the operations Register installs. The same Domain also builds the
// standalone digimon binary (see cli.NewApp), so the binary and a host share
// one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the Digimon API driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "digimon",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "digimon",
			Short:  "A command line for the Digimon API.",
			Long: `A command line for the Digimon API.

digimon reads public Digimon data from digi-api.com over plain HTTPS, shapes
it into clean records, and prints output that pipes into the rest of your
tools. No API key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/digimon-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// list: first page of Digimon stubs.
	kit.Handle(app, kit.OpMeta{Name: "list", Group: "read", List: true,
		Summary: "List Digimon", URIType: "digimon"}, listDigimon)

	// search: search by name, returns stubs.
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read",
		Summary: "Search Digimon by name",
		Args:    []kit.Arg{{Name: "query", Help: "name search query"}}}, searchDigimon)

	// digimon: full record by name or numeric ID.
	kit.Handle(app, kit.OpMeta{Name: "digimon", Group: "read", Single: true,
		Summary: "Fetch a Digimon by name or ID", URIType: "digimon", Resolver: true,
		Args: []kit.Arg{{Name: "name-or-id", Help: "digimon name (agumon) or numeric ID"}}}, getDigimon)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type listInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client *Client `kit:"inject"`
}

type searchInput struct {
	Query  string  `kit:"arg" help:"name search query"`
	Limit  int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client *Client `kit:"inject"`
}

type digimonInput struct {
	NameOrID string  `kit:"arg" help:"digimon name (agumon) or numeric ID"`
	Client   *Client `kit:"inject"`
}

// --- handlers ---

func listDigimon(ctx context.Context, in listInput, emit func(DigimonStub) error) error {
	stubs, err := in.Client.ListDigimon(ctx, in.Limit)
	if err != nil {
		return err
	}
	for _, s := range stubs {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

func searchDigimon(ctx context.Context, in searchInput, emit func(DigimonStub) error) error {
	stubs, err := in.Client.SearchDigimon(ctx, in.Query, in.Limit)
	if err != nil {
		return err
	}
	for _, s := range stubs {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

func getDigimon(ctx context.Context, in digimonInput, emit func(*Digimon) error) error {
	d, err := in.Client.GetDigimon(ctx, in.NameOrID)
	if err != nil {
		return err
	}
	return emit(d)
}

// --- Resolver: pure string functions, no network ---

// Classify turns any accepted input into the canonical (type, id) pair.
// A pure numeric string maps to ("id", input); a word-like string maps to
// ("name", input). Full digi-api.com URLs are unwrapped first.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	// Unwrap a full URL.
	if u, parseErr := url.Parse(input); parseErr == nil &&
		(u.Scheme == "http" || u.Scheme == "https") {
		// Last path segment after /digimon/ prefix.
		seg := lastSegment(u.Path)
		if seg == "" {
			return "", "", errs.Usage("unrecognized digimon reference: %q", input)
		}
		input = seg
	}

	if isNumeric(input) {
		return "id", input, nil
	}
	if isNameLike(input) {
		return "name", strings.ToLower(input), nil
	}
	return "", "", errs.Usage("unrecognized digimon reference: %q", input)
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "id", "name":
		return baseURL + "/api/v1/digimon/" + id, nil
	case "digimon":
		return baseURL + "/api/v1/digimon/" + id, nil
	default:
		return "", errs.Usage("digimon has no resource type %q", uriType)
	}
}

// --- helpers ---

// isNumeric reports whether s is a non-empty string of ASCII digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// isNameLike reports whether s looks like a Digimon name: letters, digits,
// spaces, hyphens, parentheses, dots. Empty strings are rejected.
func isNameLike(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) &&
			r != ' ' && r != '-' && r != '(' && r != ')' && r != '.' {
			return false
		}
	}
	return true
}

// lastSegment returns the last non-empty path segment of a URL path.
func lastSegment(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return ""
}

// mapErr converts library errors to the kit error kind with the right exit code.
func mapErr(err error) error {
	return err
}
