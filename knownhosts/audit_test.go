package knownhosts

import (
	"encoding/base64"
	"strings"
	"testing"
)

func auditQuery(t *testing.T, target, keyType, b64 string) Query {
	t.Helper()
	tg, err := ParseTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	return Query{Target: tg, KeyType: keyType, KeyB64: b64}
}

func TestAuditTrustedChangedUnknown(t *testing.T) {
	_, k1 := makeKey(t)
	_, k2 := makeKey(t)
	sample := Parse("host1 ssh-ed25519 " + k1 + "\n")

	r := sample.Audit(auditQuery(t, "host1", KeyTypeEd25519, k1))
	if r.Verdict != VerdictTrusted || !r.Complete {
		t.Fatalf("trusted: got %v complete=%v reason=%q", r.Verdict, r.Complete, r.Reason)
	}
	if len(r.Lines) != 1 || !r.Lines[0].SameKey || r.Lines[0].Line != 1 {
		t.Fatalf("lines = %+v", r.Lines)
	}

	r = sample.Audit(auditQuery(t, "host1", KeyTypeEd25519, k2))
	if r.Verdict != VerdictChanged {
		t.Fatalf("changed: got %v", r.Verdict)
	}

	r = sample.Audit(auditQuery(t, "other", KeyTypeEd25519, k1))
	if r.Verdict != VerdictUnknown {
		t.Fatalf("unknown: got %v", r.Verdict)
	}
}

func TestAuditRevokedBeatsEarlierNormalMatch(t *testing.T) {
	_, k1 := makeKey(t)
	// Normal match appears BEFORE the revocation of the same key.
	sample := Parse(strings.Join([]string{
		"host1 ssh-ed25519 " + k1,
		"@revoked host1 ssh-ed25519 " + k1,
	}, "\n"))
	r := sample.Audit(auditQuery(t, "host1", KeyTypeEd25519, k1))
	if r.Verdict != VerdictRevoked {
		t.Fatalf("verdict = %v, want revoked (revocation must win over earlier match)", r.Verdict)
	}
	if len(r.Lines) != 2 {
		t.Fatalf("lines = %+v", r.Lines)
	}
}

func TestAuditRevokedDifferentKeyDoesNotBlock(t *testing.T) {
	_, k1 := makeKey(t)
	_, k2 := makeKey(t)
	sample := Parse(strings.Join([]string{
		"host1 ssh-ed25519 " + k1,
		"@revoked host1 ssh-ed25519 " + k2,
	}, "\n"))
	r := sample.Audit(auditQuery(t, "host1", KeyTypeEd25519, k1))
	if r.Verdict != VerdictTrusted {
		t.Fatalf("verdict = %v, want trusted", r.Verdict)
	}
}

func TestAuditErrorsMakeSampleIncomplete(t *testing.T) {
	_, k1 := makeKey(t)
	sample := Parse(strings.Join([]string{
		"host1 ssh-ed25519 " + k1,
		"garbage line without key",
	}, "\n"))
	r := sample.Audit(auditQuery(t, "host1", KeyTypeEd25519, k1))
	if r.Verdict != VerdictInconclusive {
		t.Fatalf("verdict = %v, want inconclusive for error-containing sample", r.Verdict)
	}
	if r.Complete {
		t.Fatal("complete should be false")
	}
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Line != 2 {
		t.Fatalf("diagnostics = %+v", r.Diagnostics)
	}
}

func TestAuditCorruptMatchedLineIsDiagnosticNotCredential(t *testing.T) {
	_, k1 := makeKey(t)
	corrupt := base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 100, 1, 2})
	sample := Parse(strings.Join([]string{
		"host1 ssh-ed25519 " + corrupt,
		"host1 ssh-ed25519 " + k1,
	}, "\n"))
	r := sample.Audit(auditQuery(t, "host1", KeyTypeEd25519, k1))
	if r.Verdict != VerdictInconclusive || r.Complete {
		t.Fatalf("verdict = %v complete=%v, want inconclusive/false", r.Verdict, r.Complete)
	}
	found := false
	for _, d := range r.Diagnostics {
		if d.Line == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("corrupt line 1 should be diagnosed: %+v", r.Diagnostics)
	}
}

func TestAuditUnsupportedAndInvalidPresentedKeys(t *testing.T) {
	_, k1 := makeKey(t)
	sample := Parse("host1 ssh-ed25519 " + k1 + "\n")

	r := sample.Audit(auditQuery(t, "host1", "ssh-rsa", k1))
	if r.Verdict != VerdictUnsupported {
		t.Fatalf("rsa: verdict = %v, want unsupported", r.Verdict)
	}
	r = sample.Audit(auditQuery(t, "host1", "ssh-ed25519-cert-v01@openssh.com", k1))
	if r.Verdict != VerdictUnsupported {
		t.Fatalf("cert: verdict = %v, want unsupported", r.Verdict)
	}
	r = sample.Audit(auditQuery(t, "host1", KeyTypeEd25519, "not!base64"))
	if r.Verdict != VerdictInvalid {
		t.Fatalf("bad base64: verdict = %v, want invalid", r.Verdict)
	}
}

func TestAuditCertAuthorityLineUnsupported(t *testing.T) {
	_, k1 := makeKey(t)
	sample := Parse(strings.Join([]string{
		"@cert-authority *.example.com ssh-ed25519 " + k1,
		"host1.example.com ssh-ed25519 " + k1,
	}, "\n"))
	r := sample.Audit(auditQuery(t, "host1.example.com", KeyTypeEd25519, k1))
	if r.Verdict != VerdictInconclusive || r.Complete {
		t.Fatalf("verdict = %v complete=%v, want inconclusive/false", r.Verdict, r.Complete)
	}
}

func TestAuditPortFormsAreDistinct(t *testing.T) {
	_, k1 := makeKey(t)
	sample := Parse("[host1]:2222 ssh-ed25519 " + k1 + "\n")
	r := sample.Audit(auditQuery(t, "host1", KeyTypeEd25519, k1))
	if r.Verdict != VerdictUnknown {
		t.Fatalf("bare host must not match [host]:2222 entry: %v", r.Verdict)
	}
	r = sample.Audit(auditQuery(t, "host1:2222", KeyTypeEd25519, k1))
	if r.Verdict != VerdictTrusted {
		t.Fatalf("host1:2222 should match: %v", r.Verdict)
	}
}
