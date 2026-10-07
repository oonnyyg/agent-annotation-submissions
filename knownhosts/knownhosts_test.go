package knownhosts

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

// makeKey builds a valid ssh-ed25519 wire-format blob and its base64 form.
func makeKey(t *testing.T) (blob []byte, b64 string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blob = append(sshString(nil, KeyTypeEd25519), sshString(nil, string(pub))...)
	return blob, base64.StdEncoding.EncodeToString(blob)
}

func sshString(buf []byte, s string) []byte {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	return append(append(buf, n[:]...), s...)
}

func hashHost(t *testing.T, hostForm string) string {
	t.Helper()
	salt := make([]byte, 20)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(hostForm))
	return fmt.Sprintf("|1|%s|%s",
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

func TestParseKeepsLineNumbersAndDiagnostics(t *testing.T) {
	_, k1 := makeKey(t)
	content := strings.Join([]string{
		"# comment",
		"host1 ssh-ed25519 " + k1,
		"",
		"broken-line",
		"@revoked host2 ssh-ed25519 " + k1,
		"host3 ssh-ed25519 !!!not-base64!!!",
		"@unknown host4 ssh-ed25519 " + k1,
	}, "\n")
	s := Parse(content)
	if len(s.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(s.Entries))
	}
	if s.Entries[0].Line != 2 || s.Entries[1].Line != 5 {
		t.Fatalf("line numbers = %d,%d, want 2,5", s.Entries[0].Line, s.Entries[1].Line)
	}
	if s.Entries[1].Kind != KindRevoked {
		t.Fatalf("entry 2 kind = %v, want revoked", s.Entries[1].Kind)
	}
	if len(s.Diagnostics) != 3 {
		t.Fatalf("diagnostics = %d, want 3: %v", len(s.Diagnostics), s.Diagnostics)
	}
	wantLines := []int{4, 6, 7}
	for i, d := range s.Diagnostics {
		if d.Line != wantLines[i] {
			t.Fatalf("diagnostic %d line = %d, want %d", i, d.Line, wantLines[i])
		}
	}
}

func TestValidateKey(t *testing.T) {
	blob, b64 := makeKey(t)
	k, err := ValidateKey(KeyTypeEd25519, blob)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(k.Fingerprint, "SHA256:") {
		t.Fatalf("fingerprint = %q", k.Fingerprint)
	}
	if _, err := ValidateKey("ssh-rsa", blob); err == nil {
		t.Fatal("ssh-rsa should be rejected as unsupported")
	}
	if _, err := ValidateKey("ssh-ed25519-cert-v01@openssh.com", blob); err == nil {
		t.Fatal("cert key type should be rejected as unsupported")
	}
	// Corrupt: declared length exceeds data.
	if _, err := ValidateKey(KeyTypeEd25519, blob[:len(blob)-5]); err == nil {
		t.Fatal("truncated blob should be rejected")
	}
	// Wrong key length inside blob.
	bad := append(sshString(nil, KeyTypeEd25519), sshString(nil, "short")...)
	if _, err := ValidateKey(KeyTypeEd25519, bad); err == nil {
		t.Fatal("short key material should be rejected")
	}
	// Trailing garbage.
	if _, err := ValidateKey(KeyTypeEd25519, append(blob, 0x00)); err == nil {
		t.Fatal("trailing bytes should be rejected")
	}
	// Mismatched inner algorithm name.
	inner := append(sshString(nil, "ssh-rsa"), sshString(nil, strings.Repeat("x", 32))...)
	if _, err := ValidateKey(KeyTypeEd25519, inner); err == nil {
		t.Fatal("inner/outer type mismatch should be rejected")
	}
	_ = b64
}

func TestMatchPatterns(t *testing.T) {
	cases := []struct {
		patterns []string
		target   Target
		want     bool
	}{
		{[]string{"example.com"}, Target{"example.com", 22}, true},
		{[]string{"example.com"}, Target{"example.com", 2222}, false}, // port must not be conflated
		{[]string{"[example.com]:2222"}, Target{"example.com", 2222}, true},
		{[]string{"[example.com]:2222"}, Target{"example.com", 22}, false},
		{[]string{"*.example.com"}, Target{"a.example.com", 22}, true},
		{[]string{"*.example.com"}, Target{"example.com", 22}, false},
		{[]string{"host?.example.com"}, Target{"host1.example.com", 22}, true},
		{[]string{"a.example.com", "b.example.com"}, Target{"b.example.com", 22}, true},
		{[]string{"*.example.com", "!bad.example.com"}, Target{"bad.example.com", 22}, false},
		{[]string{"!bad.example.com", "*.example.com"}, Target{"bad.example.com", 22}, false}, // order-independent
		{[]string{"!bad.example.com", "*.example.com"}, Target{"ok.example.com", 22}, true},
	}
	for i, c := range cases {
		if got := MatchPatterns(c.patterns, c.target); got != c.want {
			t.Errorf("case %d: MatchPatterns(%v, %v) = %v, want %v", i, c.patterns, c.target, got, c.want)
		}
	}
}

func TestMatchHashedHost(t *testing.T) {
	bare := hashHost(t, "example.com")
	if !MatchPatterns([]string{bare}, Target{"example.com", 22}) {
		t.Fatal("hashed bare host should match default-port target")
	}
	if MatchPatterns([]string{bare}, Target{"example.com", 2222}) {
		t.Fatal("hashed bare host must not match non-default port")
	}
	if MatchPatterns([]string{bare}, Target{"other.com", 22}) {
		t.Fatal("hashed host must not match a different host")
	}
	bracketed := hashHost(t, "[example.com]:2222")
	if !MatchPatterns([]string{bracketed}, Target{"example.com", 2222}) {
		t.Fatal("hashed [host]:port should match the same target")
	}
	if MatchPatterns([]string{bracketed}, Target{"example.com", 22}) {
		t.Fatal("hashed [host]:port must not match default-port target")
	}
	if MatchPatterns([]string{"|1|!!!bad!!!"}, Target{"example.com", 22}) {
		t.Fatal("malformed hash pattern must not match")
	}
}

func TestParseTarget(t *testing.T) {
	cases := map[string]Target{
		"example.com":        {"example.com", 22},
		"example.com:2222":   {"example.com", 2222},
		"[example.com]:2222": {"example.com", 2222},
		"[example.com]:22":   {"example.com", 22},
	}
	for in, want := range cases {
		got, err := ParseTarget(in)
		if err != nil || got != want {
			t.Errorf("ParseTarget(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "[host]", "[host]:x", "host:0", "host:99999"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("ParseTarget(%q) should fail", bad)
		}
	}
}
