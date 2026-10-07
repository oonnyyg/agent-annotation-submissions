package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"testing"
)

// makeKey builds a valid ssh-ed25519 wire blob from a 32-byte seed.
func makeKey(seed byte) string {
	key := make([]byte, 32)
	for i := range key {
		key[i] = seed
	}
	blob := sshString("ssh-ed25519")
	blob = append(blob, sshStringBytes(key)...)
	return base64.StdEncoding.EncodeToString(blob)
}

func sshString(s string) []byte { return sshStringBytes([]byte(s)) }

func sshStringBytes(b []byte) []byte {
	out := []byte{byte(len(b) >> 24), byte(len(b) >> 16), byte(len(b) >> 8), byte(len(b))}
	return append(out, b...)
}

// hashHost builds an OpenSSH |1|salt|hash token for id.
func hashHost(id string) string {
	salt := []byte("0123456789abcdef0123")
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(id))
	return "|1|" + base64.StdEncoding.EncodeToString(salt) + "|" +
		base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

var (
	keyA = makeKey(1)
	keyB = makeKey(2)
)

func TestParseKeepsLineNumbersAndDiagnostics(t *testing.T) {
	content := "# comment\n" +
		"host1 ssh-ed25519 " + keyA + " first\n" +
		"\n" +
		"broken-line-only-one-field\n" +
		"@revoked host1 ssh-ed25519 " + keyA + "\n" +
		"host2 ssh-rsa AAAA\n" +
		"host3 ssh-ed25519-cert-v01@openssh.com AAAA\n"
	f := ParseFile(content)
	if len(f.Entries) != 4 {
		t.Fatalf("entries = %d, want 4", len(f.Entries))
	}
	if f.Entries[0].Line != 2 || f.Entries[1].Line != 5 {
		t.Fatalf("line numbers wrong: %+v", f.Entries)
	}
	if !f.Entries[1].Revoked {
		t.Fatal("revoked marker not recognized")
	}
	if len(f.Diagnostics) != 3 {
		t.Fatalf("diagnostics = %d, want 3: %+v", len(f.Diagnostics), f.Diagnostics)
	}
}

func TestDecodeKeyRejectsCorrupt(t *testing.T) {
	cases := []struct {
		name   string
		keyB64 string
	}{
		{"bad base64", "!!!"},
		{"truncated", base64.StdEncoding.EncodeToString([]byte{0, 0, 0})},
		{"wrong alg", base64.StdEncoding.EncodeToString(append(sshString("ssh-rsa"), sshStringBytes(make([]byte, 32))...))},
		{"short key", base64.StdEncoding.EncodeToString(append(sshString("ssh-ed25519"), sshStringBytes(make([]byte, 31))...))},
		{"trailing", base64.StdEncoding.EncodeToString(append(append(sshString("ssh-ed25519"), sshStringBytes(make([]byte, 32))...), 0))},
	}
	for _, c := range cases {
		e := Entry{KeyType: "ssh-ed25519", KeyB64: c.keyB64}
		if _, err := decodeKey(e); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
	if _, err := decodeKey(Entry{KeyType: "ssh-ed25519", KeyB64: keyA}); err != nil {
		t.Errorf("valid key rejected: %v", err)
	}
}

func TestHostMatching(t *testing.T) {
	tgt := Target{Host: "web.example.com", Port: 22}
	alt := Target{Host: "web.example.com", Port: 2222}
	cases := []struct {
		field string
		t     Target
		want  bool
	}{
		{"web.example.com", tgt, true},
		{"*.example.com", tgt, true},
		{"web.???????.com", tgt, true},
		{"a,web.example.com,b", tgt, true},
		{"!web.example.com,*.example.com", tgt, false},
		{"other.com", tgt, false},
		{"web.example.com", alt, false}, // default port does not match 2222
		{"[web.example.com]:2222", alt, true},
		{"[web.example.com]:2222", tgt, false},
		{hashHost("web.example.com"), tgt, true},
		{hashHost("[web.example.com]:2222"), alt, true},
		{hashHost("other.com"), tgt, false},
		{"|9|bad|hash", tgt, false},
	}
	for _, c := range cases {
		if got := hostsFieldMatches(c.field, c.t); got != c.want {
			t.Errorf("hostsFieldMatches(%q, %+v) = %v, want %v", c.field, c.t, got, c.want)
		}
	}
}
