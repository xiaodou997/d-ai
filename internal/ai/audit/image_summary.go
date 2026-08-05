package audit

import (
	"encoding/json"
	"strings"

	"xiaodou/dai/internal/ai/imageassets"
)

// SummarizeImagesResponse reduces a potentially huge OpenAI Images response
// into compact metadata suitable for long-term audit rows when raw image blobs
// are intentionally not persisted.
func SummarizeImagesResponse(body json.RawMessage) json.RawMessage {
	if len(body) == 0 {
		return nil
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	dataRaw, ok := resp["data"]
	if !ok {
		return nil
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(dataRaw, &items); err != nil {
		return nil
	}

	type itemSummary struct {
		HasB64JSON    bool   `json:"has_b64_json,omitempty"`
		HasURL        bool   `json:"has_url,omitempty"`
		URL           string `json:"url,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	}
	type summary struct {
		Created     any           `json:"created,omitempty"`
		ImageCount  int           `json:"image_count"`
		InlineCount int           `json:"inline_count"`
		URLCount    int           `json:"url_count"`
		Items       []itemSummary `json:"items,omitempty"`
	}

	out := summary{ImageCount: len(items), Items: make([]itemSummary, 0, len(items))}
	if createdRaw, ok := resp["created"]; ok {
		var created any
		if json.Unmarshal(createdRaw, &created) == nil {
			out.Created = created
		}
	}
	for _, item := range items {
		var row itemSummary
		if raw, ok := item["b64_json"]; ok {
			var value string
			if json.Unmarshal(raw, &value) == nil && value != "" {
				row.HasB64JSON = true
				out.InlineCount++
			}
		}
		if raw, ok := item["url"]; ok {
			var value string
			if json.Unmarshal(raw, &value) == nil && value != "" {
				value = strings.TrimSpace(value)
				switch {
				case imageassets.LooksLikeInlineImageValue(value):
					if !row.HasB64JSON {
						row.HasB64JSON = true
						out.InlineCount++
					}
				case imageassets.SanitizeAssetURL(value) != "":
					row.HasURL = true
					row.URL = imageassets.SanitizeAssetURL(value)
					out.URLCount++
				}
			}
		}
		if raw, ok := item["revised_prompt"]; ok {
			_ = json.Unmarshal(raw, &row.RevisedPrompt)
		}
		if row.HasB64JSON || row.HasURL || row.RevisedPrompt != "" {
			out.Items = append(out.Items, row)
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return encoded
}
