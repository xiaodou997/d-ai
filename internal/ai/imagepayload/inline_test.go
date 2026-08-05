package imagepayload

import "testing"

const testPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Jf5kAAAAASUVORK5CYII="

func TestDecodeInlineImageValueClassifiesURLAndBase64(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		handled bool
	}{
		{name: "http url", value: "https://example.test/image.png", handled: false},
		{name: "data url", value: "data:image/png;base64," + testPNGBase64, handled: true},
		{name: "raw base64", value: testPNGBase64, handled: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, contentType, handled, err := DecodeInlineImageValue(tc.value)
			if err != nil {
				t.Fatalf("DecodeInlineImageValue err = %v", err)
			}
			if handled != tc.handled {
				t.Fatalf("handled = %v, want %v", handled, tc.handled)
			}
			if handled && (len(data) == 0 || contentType != "image/png") {
				t.Fatalf("image = %d bytes %q, want PNG", len(data), contentType)
			}
		})
	}
}
