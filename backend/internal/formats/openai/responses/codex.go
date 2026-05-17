package responses

// ApplyCodexModifications strips and overrides fields in a Responses API request
// to match what the Codex upstream endpoint expects.
//
// Codex rules:
//   - Remove max_output_tokens, temperature, top_p (ignored / causes errors)
//   - Force store=false (Codex does not support persistent storage)
//   - Inject instructions if not already set
func ApplyCodexModifications(req *Request) {
	req.MaxOutputTokens = nil
	req.Temperature = nil
	req.TopP = nil

	storeFalse := false
	req.Store = &storeFalse

	if req.Instructions == "" {
		req.Instructions = "You are ChatGPT."
	}
}
