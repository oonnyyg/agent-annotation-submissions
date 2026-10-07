package main

import (
	"strings"
	"testing"
)

func sampleFile(lines ...string) *File {
	return ParseFile(strings.Join(lines, "\n"))
}

func TestTrustedMatch(t *testing.T) {
	f := sampleFile("web ssh-ed25519 " + keyA + " ok")
	r := audit(f, Target{Host: "web", Port: 22}, "ssh-ed25519", keyA)
	if r.Verdict != VerdictTrusted || r.Matches[0].Line != 1 {
		t.Fatalf("got %+v", r)
	}
	if !strings.HasPrefix(r.Fingerprint, "SHA256:") {
		t.Fatalf("bad fingerprint %q", r.Fingerprint)
	}
}

func TestRevokedWinsOverEarlierTrustedLine(t *testing.T) {
	// Trusted line comes first; revocation later in the file must
	// still reject the key.
	f := sampleFile(
		"web ssh-ed25519 "+keyA,
		"@revoked web ssh-ed25519 "+keyA,
	)
	r := audit(f, Target{Host: "web", Port: 22}, "ssh-ed25519", keyA)
	if r.Verdict != VerdictRevoked {
		t.Fatalf("verdict = %s, want revoked", r.Verdict)
	}
	if len(r.Matches) != 2 || r.Matches[0].Kind != "revoked" {
		t.Fatalf("matches = %+v", r.Matches)
	}
}

func TestKeyChangedAndFirstSeen(t *testing.T) {
	f := sampleFile("web ssh-ed25519 " + keyA)
	r := audit(f, Target{Host: "web", Port: 22}, "ssh-ed25519", keyB)
	if r.Verdict != VerdictKeyChanged {
		t.Fatalf("verdict = %s, want key-changed", r.Verdict)
	}
	r = audit(f, Target{Host: "other", Port: 22}, "ssh-ed25519", keyA)
	if r.Verdict != VerdictFirstSeen {
		t.Fatalf("verdict = %s, want first-seen", r.Verdict)
	}
}

func TestNegationExcludesOnlyItsLine(t *testing.T) {
	// Line 1 negates web but would otherwise match; line 2 is a
	// separate plain record and must still be consulted.
	f := sampleFile(
		"!web,* ssh-ed25519 "+keyB,
		"web ssh-ed25519 "+keyA,
	)
	r := audit(f, Target{Host: "web", Port: 22}, "ssh-ed25519", keyA)
	if r.Verdict != VerdictTrusted {
		t.Fatalf("verdict = %s, want trusted", r.Verdict)
	}
}

func TestCorruptAndUnsupportedEntriesAreNotCredentials(t *testing.T) {
	f := sampleFile(
		"web ssh-ed25519 !!!notbase64",
		"web ssh-rsa "+keyA,
	)
	r := audit(f, Target{Host: "web", Port: 22}, "ssh-ed25519", keyA)
	if r.Verdict != VerdictFirstSeen {
		t.Fatalf("verdict = %s, want first-seen", r.Verdict)
	}
	if len(f.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for corrupt/unsupported lines")
	}
}

func TestUnsupportedPresentedKey(t *testing.T) {
	f := sampleFile("web ssh-ed25519 " + keyA)
	r := audit(f, Target{Host: "web", Port: 22}, "ssh-rsa", keyA)
	if r.Verdict != VerdictUnsupported {
		t.Fatalf("verdict = %s, want unsupported", r.Verdict)
	}
}

func TestParseTarget(t *testing.T) {
	cases := map[string]Target{
		"web":        {Host: "web", Port: 22},
		"web:2222":   {Host: "web", Port: 2222},
		"[web]:2222": {Host: "web", Port: 2222},
	}
	for in, want := range cases {
		got, err := parseTarget(in)
		if err != nil || got != want {
			t.Errorf("parseTarget(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", ":22", "web:0", "web:99999", "[web", "web:abc"} {
		if _, err := parseTarget(bad); err == nil {
			t.Errorf("parseTarget(%q) should fail", bad)
		}
	}
}
