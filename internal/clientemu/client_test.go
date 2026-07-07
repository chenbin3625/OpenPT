package clientemu

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadClientJSONAndRenderQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qb.client")
	err := os.WriteFile(path, []byte(`{
		"keyGenerator":{"algorithm":{"type":"HASH_NO_LEADING_ZERO","length":8},"refreshOn":"TORRENT_PERSISTENT","keyCase":"upper"},
		"peerIdGenerator":{"algorithm":{"type":"REGEX","pattern":"-qB4670-[A-Za-z0-9_~\\(\\)\\!\\.\\*-]{12}"},"refreshOn":"NEVER","shouldUrlEncode":false},
		"urlEncoder":{"encodingExclusionPattern":"[A-Za-z0-9_~\\(\\)\\!\\.\\*-]","encodedHexCase":"lower"},
		"query":"info_hash={info_hash}&peer_id={peer_id}&port={port}&uploaded={uploaded}&downloaded={downloaded}&left={left}&key={key}&event={event}&numwant={numwant}",
		"numwant":200,
		"numwantOnStop":0,
		"requestHeaders":[{"name":"User-Agent","value":"qBittorrent/4.6.7"}]
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	q, err := c.RenderQuery(RenderInput{
		InfoHash:   mustHex(t, "000102030405060708090a0b0c0d0e0f10111213"),
		Uploaded:   7,
		Downloaded: 0,
		Left:       0,
		Port:       6881,
		Event:      EventStarted,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"info_hash=%00%01%02%03%04%05%06%07%08%09%0a%0b%0c%0d%0e%0f%10%11%12%13", "peer_id=-qB4670-", "uploaded=7", "downloaded=0", "left=0", "event=started", "numwant=200", "key="} {
		if !strings.Contains(q, want) {
			t.Fatalf("query %q does not contain %q", q, want)
		}
	}
	if got := c.HeadersForRequest()[0].Value; got != "qBittorrent/4.6.7" {
		t.Fatalf("header = %q", got)
	}
}

func TestURLEncoderRules(t *testing.T) {
	enc := URLEncoder{EncodingExclusionPattern: "[A-Za-z0-9-]", EncodedHexCase: "upper"}
	if err := enc.compile(); err != nil {
		t.Fatal(err)
	}
	if got := enc.EncodeString("Az-!*"); got != "Az-%21%2A" {
		t.Fatalf("EncodeString = %q", got)
	}
	if got := enc.EncodeBytes([]byte{0, 15, 255, 'A'}); got != "%00%0F%FFA" {
		t.Fatalf("EncodeBytes = %q", got)
	}
	if got := enc.EncodeString("\u008d"); got != "%8D" {
		t.Fatalf("EncodeString single-byte rune = %q, want %%8D", got)
	}
	if got := enc.EncodeString("你"); got != "%E4%BD%A0" {
		t.Fatalf("EncodeString UTF-8 rune = %q, want UTF-8 percent encoding", got)
	}
	if got := enc.EncodeString(string([]byte{0xff})); got != "%FF" {
		t.Fatalf("EncodeString invalid UTF-8 byte = %q, want %%FF", got)
	}
}

func TestHeaderPlaceholders(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv("OPENPT_JAVA_VERSION", "17.0.12")
	t.Setenv("OPENPT_OS_NAME", "Test OS")
	c, err := NewClient(ClientConfig{
		PeerGenerator: GeneratorConfig{
			Algorithm: AlgorithmConfig{Type: "REGEX", Pattern: "-AA0000-[A-Za-z0-9]{12}"},
			RefreshOn: "NEVER",
		},
		URLEncoder:     URLEncoder{EncodingExclusionPattern: "[A-Za-z0-9-]", EncodedHexCase: "lower"},
		Query:          "info_hash={infohash}&peer_id={peerid}&port={port}&uploaded={uploaded}&downloaded={downloaded}&left={left}&event={event}&numwant={numwant}",
		Numwant:        1,
		RequestHeaders: []Header{{Name: "User-Agent", Value: "Azureus;{os};Java {java}"}, {Name: "Accept-Language", Value: "{locale}"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	headers := c.HeadersForRequest()
	if strings.Contains(headers[0].Value, "{") || strings.Contains(headers[1].Value, "{") {
		t.Fatalf("placeholders were not replaced: %#v", headers)
	}
	if headers[0].Value != "Azureus;Test OS;Java 17.0.12" {
		t.Fatalf("user agent header = %q", headers[0].Value)
	}
	if headers[1].Value != "zh-CN" {
		t.Fatalf("locale header = %q", headers[1].Value)
	}
}

func TestNumwantOnStopMustNotBeNegative(t *testing.T) {
	_, err := NewClient(ClientConfig{
		PeerGenerator: GeneratorConfig{
			Algorithm: AlgorithmConfig{Type: "REGEX", Pattern: "-AA0000-[A-Za-z0-9]{12}"},
			RefreshOn: "NEVER",
		},
		URLEncoder:    URLEncoder{EncodingExclusionPattern: "[A-Za-z0-9-]", EncodedHexCase: "lower"},
		Query:         "info_hash={infohash}&peer_id={peerid}&port={port}&uploaded={uploaded}&downloaded={downloaded}&left={left}&event={event}&numwant={numwant}",
		Numwant:       1,
		NumwantOnStop: -1,
	})
	if err == nil {
		t.Fatal("expected negative numwantOnStop to fail")
	}
}

func TestGeneratorRejectsInvalidRefreshOn(t *testing.T) {
	_, err := NewGenerator(GeneratorConfig{
		Algorithm: AlgorithmConfig{Type: "REGEX", Pattern: "-AA0000-[A-Za-z0-9]{12}"},
		RefreshOn: "TYPO",
	}, true)
	if err == nil {
		t.Fatal("expected invalid refreshOn to fail")
	}
}

func TestHashLengthMustBePositive(t *testing.T) {
	_, err := NewAlgorithm(AlgorithmConfig{Type: "HASH", Length: 0})
	if err == nil {
		t.Fatal("expected HASH length 0 to fail")
	}
}

func TestPeerIDMustBeTwentyBytes(t *testing.T) {
	_, err := NewClient(ClientConfig{
		PeerGenerator: GeneratorConfig{
			Algorithm: AlgorithmConfig{Type: "REGEX", Pattern: "-AA0000-[A-Za-z0-9]{11}"},
			RefreshOn: "NEVER",
		},
		URLEncoder: URLEncoder{EncodingExclusionPattern: "[A-Za-z0-9-]", EncodedHexCase: "lower"},
		Query:      "info_hash={infohash}&peer_id={peerid}&port={port}&uploaded={uploaded}&downloaded={downloaded}&left={left}&event={event}&numwant={numwant}",
		Numwant:    1,
	})
	if err == nil {
		t.Fatal("expected short peer_id generator to fail")
	}
}

func TestRandFailurePanics(t *testing.T) {
	old := cryptoRandReader
	cryptoRandReader = errReader{}
	defer func() { cryptoRandReader = old }()
	defer func() {
		if recover() == nil {
			t.Fatal("expected crypto/rand failure to panic")
		}
	}()
	_ = randInt(16)
}

func TestGeneratorRefreshPolicies(t *testing.T) {
	never, err := NewGenerator(GeneratorConfig{
		Algorithm: AlgorithmConfig{Type: "REGEX", Pattern: "-AA0000-[A-Za-z0-9]{12}"},
		RefreshOn: "NEVER",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := never.Get("a", EventStarted), never.Get("b", EventStarted); a != b {
		t.Fatalf("NEVER generated different peer ids: %q != %q", a, b)
	}

	volatile, err := NewGenerator(GeneratorConfig{
		Algorithm: AlgorithmConfig{Type: "REGEX", Pattern: "-BB0000-[A-Za-z0-9]{12}"},
		RefreshOn: "TORRENT_VOLATILE",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	first := volatile.Get("hash", EventStarted)
	if got := volatile.Get("hash", EventNone); got != first {
		t.Fatalf("volatile changed before stop: %q != %q", got, first)
	}
	_ = volatile.Get("hash", EventStopped)
	if got := volatile.Get("hash", EventStarted); got == first {
		t.Fatalf("volatile did not refresh after stop")
	}

	timed, err := NewGenerator(GeneratorConfig{
		Algorithm:    AlgorithmConfig{Type: "HASH", Length: 8},
		RefreshOn:    "TIMED",
		RefreshEvery: 1,
		KeyCase:      "lower",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	a := timed.Get("hash", EventNone)
	if b := timed.Get("hash", EventNone); a != b {
		t.Fatalf("timed refreshed too early: %q != %q", a, b)
	}
	time.Sleep(1100 * time.Millisecond)
	if b := timed.Get("hash", EventNone); a == b {
		t.Fatalf("timed did not refresh after interval")
	}
}

func TestRandomPoolWithChecksum(t *testing.T) {
	alg := randomPoolChecksumAlgorithm{prefix: "-TR3000-", pool: []rune("0123456789abcdefghijklmnopqrstuvwxyz"), base: 36}
	got := alg.Generate()
	if len(got) != 20 || !strings.HasPrefix(got, "-TR3000-") {
		t.Fatalf("peer id = %q", got)
	}
	total := 0
	for _, ch := range got[len("-TR3000-"):] {
		idx := strings.IndexRune("0123456789abcdefghijklmnopqrstuvwxyz", ch)
		if idx < 0 {
			t.Fatalf("unexpected char %q in %q", ch, got)
		}
		total += idx
	}
	if total%36 != 0 {
		t.Fatalf("checksum total %% 36 = %d", total%36)
	}
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("boom")
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
