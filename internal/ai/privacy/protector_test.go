package privacy

import (
	"bytes"
	"testing"
)

func TestProtectorRoundTripJSON(t *testing.T) {
	p := NewProtector()
	protected, mapping, err := p.RedactJSON([]byte(`{"messages":[{"content":"联系 alice@example.com 或 13800138000"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if mapping == nil || bytes.Contains(protected, []byte("alice@example.com")) || bytes.Contains(protected, []byte("13800138000")) {
		t.Fatalf("PII was not redacted: %s", protected)
	}
	restored := p.RestoreJSON(protected, mapping)
	if !bytes.Contains(restored, []byte("alice@example.com")) || !bytes.Contains(restored, []byte("13800138000")) {
		t.Fatalf("PII was not restored: %s", restored)
	}
}

func TestProtectorRedactsBearerText(t *testing.T) {
	p := NewProtector()
	protected, mapping, err := p.RedactJSON([]byte(`{"text":"Bearer abcdefghijklmnop"}`))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(protected, []byte("abcdefghijklmnop")) {
		t.Fatalf("bearer token remained: %s", protected)
	}
	if got := string(p.RestoreJSON(protected, mapping)); !bytes.Contains([]byte(got), []byte("abcdefghijklmnop")) {
		t.Fatalf("token was not restored: %s", got)
	}
}

func TestProtectorRoundTripNonJSONText(t *testing.T) {
	p := NewProtector()
	protected, mapping, err := p.RedactJSON([]byte("contact alice@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(protected, []byte("alice@example.com")) {
		t.Fatalf("PII was not redacted: %s", protected)
	}
	if restored := p.RestoreJSON(protected, mapping); !bytes.Contains(restored, []byte("alice@example.com")) {
		t.Fatalf("PII was not restored: %s", restored)
	}
}
