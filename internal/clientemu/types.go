package clientemu

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Event string

const (
	EventNone    Event = ""
	EventStarted Event = "started"
	EventStopped Event = "stopped"
)

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ClientConfig struct {
	KeyGenerator   *GeneratorConfig `json:"keyGenerator"`
	PeerGenerator  GeneratorConfig  `json:"peerIdGenerator"`
	URLEncoder     URLEncoder       `json:"urlEncoder"`
	Query          string           `json:"query"`
	Numwant        int              `json:"numwant"`
	NumwantOnStop  int              `json:"numwantOnStop"`
	RequestHeaders []Header         `json:"requestHeaders"`
}

type GeneratorConfig struct {
	Algorithm       AlgorithmConfig `json:"algorithm"`
	RefreshOn       string          `json:"refreshOn"`
	RefreshEvery    int             `json:"refreshEvery"`
	ShouldURLEncode bool            `json:"shouldUrlEncode"`
	KeyCase         string          `json:"keyCase"`
}

type AlgorithmConfig struct {
	Type                string `json:"type"`
	Pattern             string `json:"pattern"`
	Length              int    `json:"length"`
	InclusiveLowerBound int64  `json:"inclusiveLowerBound"`
	InclusiveUpperBound int64  `json:"inclusiveUpperBound"`
	Prefix              string `json:"prefix"`
	CharactersPool      string `json:"charactersPool"`
	Base                int    `json:"base"`
}

type Client struct {
	Query         string
	Headers       []Header
	Numwant       int
	NumwantOnStop int
	Encoder       URLEncoder
	peer          *Generator
	key           *Generator
	PeerURLEncode bool
}

func LoadClient(path string) (*Client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ClientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return NewClient(cfg)
}

func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Query == "" {
		return nil, errors.New("client query is required")
	}
	if cfg.Numwant < 1 {
		return nil, errors.New("numwant must be at least 1")
	}
	peer, err := NewGenerator(cfg.PeerGenerator, true)
	if err != nil {
		return nil, fmt.Errorf("peerIdGenerator: %w", err)
	}
	var key *Generator
	if cfg.KeyGenerator != nil {
		key, err = NewGenerator(*cfg.KeyGenerator, false)
		if err != nil {
			return nil, fmt.Errorf("keyGenerator: %w", err)
		}
	} else if strings.Contains(cfg.Query, "{key}") {
		return nil, errors.New("query contains {key}, but keyGenerator is missing")
	}
	if cfg.URLEncoder.EncodingExclusionPattern == "" {
		return nil, errors.New("urlEncoder.encodingExclusionPattern is required")
	}
	if cfg.URLEncoder.EncodedHexCase == "" {
		cfg.URLEncoder.EncodedHexCase = "lower"
	}
	if err := cfg.URLEncoder.compile(); err != nil {
		return nil, err
	}
	headers := make([]Header, len(cfg.RequestHeaders))
	for i, h := range cfg.RequestHeaders {
		headers[i] = Header{Name: h.Name, Value: replaceHeaderPlaceholders(h.Value)}
	}
	return &Client{
		Query:         collapseAmpersands(cfg.Query),
		Headers:       headers,
		Numwant:       cfg.Numwant,
		NumwantOnStop: cfg.NumwantOnStop,
		Encoder:       cfg.URLEncoder,
		peer:          peer,
		key:           key,
		PeerURLEncode: cfg.PeerGenerator.ShouldURLEncode,
	}, nil
}

func replaceHeaderPlaceholders(value string) string {
	value = strings.ReplaceAll(value, "{java}", strings.TrimPrefix(runtime.Version(), "go"))
	value = strings.ReplaceAll(value, "{os}", runtime.GOOS)
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	if locale == "" {
		locale = "en-US"
	}
	value = strings.ReplaceAll(value, "{locale}", locale)
	return value
}

type RenderInput struct {
	InfoHash   []byte
	InfoHashID string
	Uploaded   int64
	Downloaded int64
	Left       int64
	Port       int
	Event      Event
	IP         string
	IPv6       string
}

func (c *Client) RenderQuery(in RenderInput) (string, error) {
	id := in.InfoHashID
	if id == "" {
		id = string(in.InfoHash)
	}
	q := c.Query
	repl := map[string]string{
		"{infohash}":   c.Encoder.EncodeBytes(in.InfoHash),
		"{info_hash}":  c.Encoder.EncodeBytes(in.InfoHash),
		"{uploaded}":   strconv.FormatInt(in.Uploaded, 10),
		"{downloaded}": strconv.FormatInt(in.Downloaded, 10),
		"{left}":       strconv.FormatInt(in.Left, 10),
		"{port}":       strconv.Itoa(in.Port),
		"{numwant}":    strconv.Itoa(c.numwant(in.Event)),
	}
	for k, v := range repl {
		q = strings.ReplaceAll(q, k, v)
	}
	peer := c.peer.Get(id, in.Event)
	if c.PeerURLEncode {
		peer = c.Encoder.EncodeString(peer)
	}
	q = strings.ReplaceAll(q, "{peerid}", peer)
	q = strings.ReplaceAll(q, "{peer_id}", peer)
	if in.IP != "" {
		q = strings.ReplaceAll(q, "{ip}", in.IP)
	}
	if in.IPv6 != "" {
		q = strings.ReplaceAll(q, "{ipv6}", c.Encoder.EncodeString(in.IPv6))
	}
	q = removeEmptyParam(q, "{ip}")
	q = removeEmptyParam(q, "{ipv6}")
	if in.Event == EventNone {
		q = removeEmptyParam(q, "{event}")
	} else {
		q = strings.ReplaceAll(q, "{event}", string(in.Event))
	}
	if strings.Contains(q, "{key}") {
		if c.key == nil {
			return "", errors.New("query contains {key}, but no key generator exists")
		}
		q = strings.ReplaceAll(q, "{key}", c.Encoder.EncodeString(c.key.Get(id, in.Event)))
	}
	if m := regexp.MustCompile(`\{.*?\}`).FindString(q); m != "" {
		return "", fmt.Errorf("unrecognized client placeholder %s", m)
	}
	q = collapseAmpersands(q)
	q = strings.Trim(q, "&")
	return q, nil
}

func (c *Client) HeadersForRequest() []Header {
	out := make([]Header, len(c.Headers))
	copy(out, c.Headers)
	return out
}

func (c *Client) numwant(event Event) int {
	if event == EventStopped {
		return c.NumwantOnStop
	}
	return c.Numwant
}

func removeEmptyParam(q, placeholder string) string {
	parts := strings.Split(q, "&")
	kept := parts[:0]
	for _, p := range parts {
		if !strings.Contains(p, placeholder) {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "&")
}

func collapseAmpersands(s string) string {
	for strings.Contains(s, "&&") {
		s = strings.ReplaceAll(s, "&&", "&")
	}
	return s
}

type Generator struct {
	mu              sync.Mutex
	alg             Algorithm
	refreshOn       string
	refreshEvery    time.Duration
	keyCase         string
	isPeer          bool
	globalValue     string
	lastGeneration  time.Time
	perTorrent      map[string]string
	perTorrentTouch map[string]time.Time
}

func NewGenerator(cfg GeneratorConfig, isPeer bool) (*Generator, error) {
	alg, err := NewAlgorithm(cfg.Algorithm)
	if err != nil {
		return nil, err
	}
	if cfg.RefreshOn == "" {
		cfg.RefreshOn = "NEVER"
	}
	g := &Generator{
		alg:             alg,
		refreshOn:       cfg.RefreshOn,
		refreshEvery:    time.Duration(cfg.RefreshEvery) * time.Second,
		keyCase:         cfg.KeyCase,
		isPeer:          isPeer,
		perTorrent:      map[string]string{},
		perTorrentTouch: map[string]time.Time{},
	}
	if (cfg.RefreshOn == "TIMED" || cfg.RefreshOn == "TIMED_OR_AFTER_STARTED_ANNOUNCE") && cfg.RefreshEvery < 1 {
		return nil, errors.New("refreshEvery must be greater than 0")
	}
	if cfg.RefreshOn == "NEVER" {
		g.globalValue = g.generate()
	}
	return g, nil
}

func (g *Generator) Get(infoHash string, event Event) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	switch g.refreshOn {
	case "ALWAYS":
		return g.generate()
	case "TIMED":
		if g.globalValue == "" || now.Sub(g.lastGeneration) >= g.refreshEvery {
			g.globalValue = g.generate()
			g.lastGeneration = now
		}
		return g.globalValue
	case "TIMED_OR_AFTER_STARTED_ANNOUNCE":
		if g.globalValue == "" || now.Sub(g.lastGeneration) >= g.refreshEvery {
			g.globalValue = g.generate()
			g.lastGeneration = now
		}
		v := g.globalValue
		if event == EventStarted {
			g.globalValue = g.generate()
			g.lastGeneration = now
		}
		return v
	case "TORRENT_VOLATILE":
		v, ok := g.perTorrent[infoHash]
		if !ok {
			v = g.generate()
			g.perTorrent[infoHash] = v
		}
		if event == EventStopped {
			delete(g.perTorrent, infoHash)
			delete(g.perTorrentTouch, infoHash)
		}
		return v
	case "TORRENT_PERSISTENT":
		v, ok := g.perTorrent[infoHash]
		if !ok {
			v = g.generate()
			g.perTorrent[infoHash] = v
		}
		g.perTorrentTouch[infoHash] = now
		for k, touched := range g.perTorrentTouch {
			if now.Sub(touched) >= 120*time.Minute {
				delete(g.perTorrent, k)
				delete(g.perTorrentTouch, k)
			}
		}
		return v
	case "NEVER":
		fallthrough
	default:
		if g.globalValue == "" {
			g.globalValue = g.generate()
		}
		return g.globalValue
	}
}

func (g *Generator) generate() string {
	v := g.alg.Generate()
	if !g.isPeer {
		switch strings.ToLower(g.keyCase) {
		case "upper":
			v = strings.ToUpper(v)
		case "lower":
			v = strings.ToLower(v)
		}
	}
	return v
}

type Algorithm interface {
	Generate() string
}

func NewAlgorithm(cfg AlgorithmConfig) (Algorithm, error) {
	switch cfg.Type {
	case "HASH":
		return hashAlgorithm{length: cfg.Length, noLeadingZero: false}, nil
	case "HASH_NO_LEADING_ZERO":
		return hashAlgorithm{length: cfg.Length, noLeadingZero: true}, nil
	case "DIGIT_RANGE_TRANSFORMED_TO_HEX_WITHOUT_LEADING_ZEROES":
		if cfg.InclusiveUpperBound < cfg.InclusiveLowerBound {
			return nil, errors.New("inclusiveUpperBound must be greater than inclusiveLowerBound")
		}
		return digitHexAlgorithm{min: cfg.InclusiveLowerBound, max: cfg.InclusiveUpperBound}, nil
	case "RANDOM_POOL_WITH_CHECKSUM":
		return randomPoolChecksumAlgorithm{prefix: cfg.Prefix, pool: []rune(cfg.CharactersPool), base: cfg.Base}, nil
	case "REGEX":
		return newRegexAlgorithm(cfg.Pattern)
	default:
		return nil, fmt.Errorf("unsupported algorithm type %q", cfg.Type)
	}
}

type hashAlgorithm struct {
	length        int
	noLeadingZero bool
}

func (a hashAlgorithm) Generate() string {
	const hex = "0123456789ABCDEF"
	out := make([]byte, a.length)
	for i := range out {
		out[i] = hex[randInt(16)]
	}
	s := string(out)
	if a.noLeadingZero {
		s = strings.TrimLeft(s, "0")
	}
	return s
}

type digitHexAlgorithm struct {
	min, max int64
}

func (a digitHexAlgorithm) Generate() string {
	n := randInt64(a.max-a.min+1) + a.min
	return strconv.FormatInt(n, 16)
}

type randomPoolChecksumAlgorithm struct {
	prefix string
	pool   []rune
	base   int
}

func (a randomPoolChecksumAlgorithm) Generate() string {
	suffixLen := 20 - len([]rune(a.prefix))
	if suffixLen <= 0 || len(a.pool) == 0 || a.base <= 0 {
		return a.prefix
	}
	buf := make([]rune, suffixLen)
	total := 0
	for i := 0; i < suffixLen-1; i++ {
		v := randInt(a.base)
		total += v
		buf[i] = a.pool[v%len(a.pool)]
	}
	check := 0
	if total%a.base != 0 {
		check = a.base - (total % a.base)
	}
	buf[suffixLen-1] = a.pool[check%len(a.pool)]
	return a.prefix + string(buf)
}

func randInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func randInt64(max int64) int64 {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0
	}
	return n.Int64()
}
