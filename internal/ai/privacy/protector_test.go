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

func TestProtectorUsesConfiguredRulesAndPrefix(t *testing.T) {
	protector, err := NewProtectorWithConfig(Config{
		PlaceholderPrefix: "secure",
		Rules: []RuleConfig{
			{ID: "employee_id", Name: "员工编号", Pattern: `EMP-[0-9]{4}`, Enabled: true},
			{ID: "email", Name: "邮箱", Pattern: `[^ ]+@[^ ]+`, Enabled: false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	protected, mapping := protector.RedactText([]byte("EMP-1234 alice@example.com"))
	if got := string(protected); got != "__SECURE_PII_EMPLOYEE_ID_1__ alice@example.com" {
		t.Fatalf("unexpected protected text: %s", got)
	}
	if got := string(protector.RestoreText(protected, mapping)); got != "EMP-1234 alice@example.com" {
		t.Fatalf("unexpected restored text: %s", got)
	}
}

func TestValidateConfigRejectsInvalidRules(t *testing.T) {
	_, err := ValidateConfig(Config{
		PlaceholderPrefix: "DAI",
		Rules:             []RuleConfig{{ID: "broken", Name: "无效规则", Pattern: "("}},
	})
	if err == nil {
		t.Fatal("expected invalid regular expression to fail validation")
	}
	_, err = ValidateConfig(Config{PlaceholderPrefix: "bad-prefix!"})
	if err == nil {
		t.Fatal("expected invalid placeholder prefix to fail validation")
	}
}
