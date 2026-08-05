package imagepayload

const (
	// MaxImageRequestBodyBytes is the platform-wide HTTP body limit for image requests.
	MaxImageRequestBodyBytes int64 = 64 << 20
	// MaxImageRawInputBytes is the platform-wide decoded limit for one input image.
	MaxImageRawInputBytes int64 = 32 << 20
)
