package riskcontrol

import (
	"bytes"
	"mime/multipart"
	"strings"
	"testing"
)

func TestExtractImagePromptJSON(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "生图请求",
			body: `{"model":"gpt-image-2","prompt":"一只白色陶瓷杯","n":1,"size":"1024x1024"}`,
			want: "一只白色陶瓷杯",
		},
		{
			// creative-service 的修图请求形状：基线图 + 蒙版 + 合成指令。
			name: "修图请求（JSON 传输）",
			body: `{"model":"gpt-image-2","prompt":"把杯身改成青色","images":[{"image_url":"data:image/png;base64,AAA"}],"mask":{"image_url":"data:image/png;base64,BBB"}}`,
			want: "把杯身改成青色",
		},
		{"缺 prompt", `{"model":"gpt-image-2","n":1}`, ""},
		{"prompt 非字符串", `{"prompt":{"text":"x"}}`, ""},
		{"空 prompt", `{"prompt":"   "}`, ""},
		{"非法 JSON", `not json at all`, ""},
		{"空体", ``, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractImagePrompt([]byte(tc.body), "application/json"); got != tc.want {
				t.Fatalf("prompt = %q, want %q", got, tc.want)
			}
		})
	}
}

// OpenAI 原生 images.edit 走 multipart：prompt 与图片文件混在同一个表单里。
func TestExtractImagePromptMultipart(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	// 图片 part 先于 prompt，确保提取要真的走完前面的文件 part。
	imagePart, _ := w.CreateFormFile("image", "base.png")
	_, _ = imagePart.Write(bytes.Repeat([]byte{0x89, 0x50, 0x4e, 0x47}, 1024))
	_ = w.WriteField("model", "gpt-image-2")
	_ = w.WriteField("prompt", "把背景换成纯白")
	maskPart, _ := w.CreateFormFile("mask", "mask.png")
	_, _ = maskPart.Write(bytes.Repeat([]byte{0x89, 0x50, 0x4e, 0x47}, 512))
	_ = w.Close()

	got := ExtractImagePrompt(buf.Bytes(), w.FormDataContentType())
	if got != "把背景换成纯白" {
		t.Fatalf("prompt = %q, want 把背景换成纯白", got)
	}
}

func TestExtractImagePromptMultipartWithoutPrompt(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("image", "base.png")
	_, _ = part.Write([]byte{0x89, 0x50})
	_ = w.Close()
	if got := ExtractImagePrompt(buf.Bytes(), w.FormDataContentType()); got != "" {
		t.Fatalf("prompt = %q, want empty", got)
	}
}

// 边界坏掉时不能 panic，也不能把整段二进制当成待审文本。
func TestExtractImagePromptBrokenMultipart(t *testing.T) {
	if got := ExtractImagePrompt([]byte("--nope\r\nbroken"), "multipart/form-data"); got != "" {
		t.Fatalf("prompt = %q, want empty for missing boundary", got)
	}
	if got := ExtractImagePrompt([]byte("garbage"), "multipart/form-data; boundary=xyz"); got != "" {
		t.Fatalf("prompt = %q, want empty for unparsable body", got)
	}
}

// 截断等于绕过：关键词可能落在提示词末尾。
func TestExtractImagePromptIsNotTruncated(t *testing.T) {
	long := strings.Repeat("很长的描述", 500) + "badword"
	body := `{"prompt":"` + long + `"}`
	got := ExtractImagePrompt([]byte(body), "application/json")
	if !strings.HasSuffix(got, "badword") {
		t.Fatalf("prompt tail lost; len=%d", len([]rune(got)))
	}
}

// 以文件形式提交的 prompt part 不读：imageedit 解码器会以「unsupported multipart file
// field」拒绝这类请求，它到不了上游；而审核跑在解码之前，读它只是把一份任意大的上传
// 缓冲进内存。
func TestExtractImagePromptSkipsPromptSubmittedAsFile(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	filePart, _ := w.CreateFormFile("prompt", "prompt.txt")
	_, _ = filePart.Write([]byte("badword"))
	_ = w.WriteField("model", "gpt-image-2")
	_ = w.Close()

	if got := ExtractImagePrompt(buf.Bytes(), w.FormDataContentType()); got != "" {
		t.Fatalf("prompt = %q, want empty (file-shaped prompt part is not a prompt)", got)
	}
}
