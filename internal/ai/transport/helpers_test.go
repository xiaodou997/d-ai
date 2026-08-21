package transport

import "testing"

func TestParseTransportUUID(t *testing.T) {
	const canonical = "10000000-0000-0000-0000-0000000000ab"
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "canonical", value: canonical, want: canonical},
		{name: "uppercase", value: "10000000-0000-0000-0000-0000000000AB", want: canonical},
		{name: "compact", value: "100000000000000000000000000000ab", want: canonical},
		{name: "empty"},
		{name: "urn", value: "urn:uuid:" + canonical},
		{name: "braced", value: "{" + canonical + "}"},
		{name: "wrong separators", value: "10000000_0000_0000_0000_0000000000ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTransportUUID(tt.value)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("parseTransportUUID(%q) = %s, want error", tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTransportUUID(%q): %v", tt.value, err)
			}
			if got.String() != tt.want {
				t.Fatalf("parseTransportUUID(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
