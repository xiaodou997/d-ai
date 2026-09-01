package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"strings"

	"github.com/jackc/pgx/v5"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/ai/imageedit"
)

// consoleImageResolution is deliberately local to the caller's goroutine.
// Only Input and ModelCode are persisted; Replay is discarded after Prepare
// and rebuilt from the stored input by every worker attempt.
type consoleImageResolution struct {
	Input     json.RawMessage
	ModelCode string
	Replay    gateway.ReplayInput
}

func (s *Console) prepareConsoleImageTask(
	ctx context.Context,
	subject coreidentity.Subject,
	body []byte,
	contentType string,
	operation string,
) (consoleImageResolution, error) {
	input, err := decodeConsoleImageSubmission(body, contentType, operation)
	if err != nil {
		return consoleImageResolution{}, err
	}
	return s.resolveConsoleImage(ctx, &subject, input, true)
}

func (s *Console) resolveConsoleImageTask(
	ctx context.Context,
	subject coreidentity.Subject,
	input consoleImageTaskInputPayload,
	operation string,
) (gateway.ReplayInput, error) {
	if input.Operation == "" {
		input.Operation = operation
	}
	if input.Operation != operation {
		return gateway.ReplayInput{}, fmt.Errorf("console image task operation %q does not match handler %q", input.Operation, operation)
	}
	return s.resolve(ctx, &subject, input)
}

func decodeConsoleImageSubmission(body []byte, contentType, operation string) (consoleImageTaskInputPayload, error) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == imageedit.TransportMultipart {
		if operation != "edit" {
			return consoleImageTaskInputPayload{}, domain.NewValidationError("operation", "multipart requests only support image edit")
		}
		return decodeConsoleImageMultipartSubmission(body, contentType)
	}

	var req consoleImageGenerateRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return consoleImageTaskInputPayload{}, domain.NewValidationError("request", "invalid request body: "+err.Error())
	}
	requestOperation := strings.TrimSpace(req.Operation)
	if requestOperation == "" {
		requestOperation = "generation"
	}
	if requestOperation != operation {
		return consoleImageTaskInputPayload{}, domain.NewValidationError("operation", "operation must be generation or edit")
	}
	return consoleImageInputFromRequest(requestOperation, req, imageedit.Request{})
}

func decodeConsoleImageMultipartSubmission(body []byte, contentType string) (consoleImageTaskInputPayload, error) {
	editRequest, err := imageedit.Decode(body, contentType)
	if err != nil {
		return consoleImageTaskInputPayload{}, domain.NewValidationError("request", err.Error())
	}
	fields, err := formats.MultipartScalarFields(body, contentType, 1<<20)
	if err != nil {
		return consoleImageTaskInputPayload{}, domain.NewValidationError("request", err.Error())
	}
	req := consoleImageGenerateRequest{
		Operation:      "edit",
		ModelCode:      strings.TrimSpace(fields["model"]),
		GroupID:        strings.TrimSpace(fields["group_id"]),
		Prompt:         strings.TrimSpace(fields["prompt"]),
		Size:           strings.TrimSpace(fields["size"]),
		ResponseFormat: strings.TrimSpace(fields["response_format"]),
		Background:     strings.TrimSpace(fields["background"]),
		InputFidelity:  strings.TrimSpace(fields["input_fidelity"]),
		Moderation:     strings.TrimSpace(fields["moderation"]),
		OutputFormat:   strings.TrimSpace(fields["output_format"]),
		User:           strings.TrimSpace(fields["user"]),
		N:              editRequest.N,
	}
	if editRequest.OutputCompression != nil {
		req.OutputCompression = editRequest.OutputCompression
	}
	return consoleImageInputFromRequest("edit", req, editRequest)
}

func consoleImageInputFromRequest(operation string, req consoleImageGenerateRequest, editRequest imageedit.Request) (consoleImageTaskInputPayload, error) {
	var (
		raw []byte
		err error
	)
	if operation == "edit" {
		if len(editRequest.Images) == 0 {
			editRequest, err = decodeConsoleImageEditRequest(req)
			if err != nil {
				return consoleImageTaskInputPayload{}, domain.NewValidationError("request", err.Error())
			}
		}
		raw, err = buildConsoleImageEditTaskInput(req, editRequest.Mask != nil, editRequest)
		if err != nil {
			return consoleImageTaskInputPayload{}, domain.NewValidationError("request", err.Error())
		}
	} else {
		raw = buildConsoleImageTaskInput("generation", req, false, false)
	}
	var input consoleImageTaskInputPayload
	if err := json.Unmarshal(raw, &input); err != nil {
		return consoleImageTaskInputPayload{}, fmt.Errorf("decode canonical console image input: %w", err)
	}
	return input, nil
}

// resolve is the task execution path: persisted, redacted input in; one
// synthesized runtime call out. Submit uses the same resolver through
// resolveConsoleImage and discards Replay, forcing every worker to rebuild it.
func (s *Console) resolve(
	ctx context.Context,
	subject *coreidentity.Subject,
	input consoleImageTaskInputPayload,
) (gateway.ReplayInput, error) {
	resolved, err := s.resolveConsoleImage(ctx, subject, input, true)
	return resolved.Replay, err
}

func (s *Console) resolveConsoleImage(
	ctx context.Context,
	subject *coreidentity.Subject,
	input consoleImageTaskInputPayload,
	autoStream bool,
) (consoleImageResolution, error) {
	if subject == nil {
		return consoleImageResolution{}, errors.New("console image subject is required")
	}
	req := consoleImageRequestFromTaskInput(input, "")
	operation := strings.TrimSpace(input.Operation)
	if operation == "" {
		operation = "generation"
	}
	if operation != "generation" && operation != "edit" {
		return consoleImageResolution{}, domain.NewValidationError("operation", "operation must be generation or edit")
	}
	req.Operation = operation
	req.ModelCode = strings.TrimSpace(req.ModelCode)
	req.GroupID = strings.TrimSpace(req.GroupID)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.ModelCode == "" {
		return consoleImageResolution{}, domain.NewValidationError("model", "model is required")
	}
	if req.Prompt == "" {
		return consoleImageResolution{}, domain.NewValidationError("prompt", "prompt is required")
	}

	modelCode := req.ModelCode
	groupID := req.GroupID

	req.ModelCode = modelCode
	req.GroupID = groupID
	runtimeSubject := consoleSubjectForSession(subject, groupID)
	if err := s.ensureConsoleImageModelGranted(ctx, runtimeSubject, modelCode); err != nil {
		return consoleImageResolution{}, err
	}
	policy, err := s.consoleImagePolicyForModel(ctx, runtimeSubject, modelCode)
	if err != nil {
		return consoleImageResolution{}, err
	}
	applyConsoleImagePolicy(&req, policy, autoStream)

	var (
		body        []byte
		contentType = imageedit.TransportJSON
		inputRaw    []byte
		clientPath  string
	)
	if operation == "generation" {
		body, err = buildConsoleImageBody(req, modelCode, req.Prompt)
		if err != nil {
			return consoleImageResolution{}, domain.NewValidationError("request", err.Error())
		}
		inputRaw = buildConsoleImageTaskInput(operation, req, false, false)
		clientPath = "/v1/images/generations"
	} else {
		editRequest, err := decodePersistedConsoleImageEditRequestFromPayload(input)
		if err != nil {
			return consoleImageResolution{}, domain.NewValidationError("request", err.Error())
		}
		editRequest.Model = modelCode
		editRequest.Prompt = req.Prompt
		editRequest.ResponseFormat = req.ResponseFormat
		editRequest.Stream = consoleImageRequestStreamEnabled(req)
		editRequest.N = req.N
		body, err = imageedit.CanonicalJSON(editRequest)
		if err != nil {
			return consoleImageResolution{}, domain.NewValidationError("request", err.Error())
		}
		inputRaw, err = buildConsoleImageEditTaskInput(req, editRequest.Mask != nil, editRequest)
		if err != nil {
			return consoleImageResolution{}, domain.NewValidationError("request", err.Error())
		}
		clientPath = "/v1/images/edits"
	}

	replaySubject := *runtimeSubject
	replaySubject.RequestSource = coreidentity.RequestSourceWebImage
	return consoleImageResolution{
		Input:     json.RawMessage(inputRaw),
		ModelCode: modelCode,
		Replay: gateway.ReplayInput{
			Subject:        replaySubject,
			Capability:     domain.CapabilityImage,
			Protocol:       domain.ProtocolOpenAIImages,
			ClientPath:     clientPath,
			Body:           body,
			ContentType:    contentType,
			StreamExpected: consoleImageRequestStreamEnabled(req),
		},
	}, nil
}

func decodePersistedConsoleImageEditRequestFromPayload(input consoleImageTaskInputPayload) (imageedit.Request, error) {
	if len(bytes.TrimSpace(input.EditRequest)) == 0 {
		return imageedit.Request{}, errors.New("persisted image edit request is missing")
	}
	req, err := imageedit.Decode(input.EditRequest, imageedit.TransportJSON)
	if err != nil {
		return imageedit.Request{}, fmt.Errorf("decode persisted image edit request: %w", err)
	}
	return req, nil
}

type consoleImageGenerateRequest struct {
	Operation         string               `json:"operation,omitempty"`
	ModelCode         string               `json:"model"`
	GroupID           string               `json:"group_id,omitempty"`
	Prompt            string               `json:"prompt"`
	N                 int                  `json:"n,omitempty"`
	Images            []consoleImageSource `json:"images,omitempty"`
	Mask              *consoleImageSource  `json:"mask,omitempty"`
	Size              string               `json:"size,omitempty"`
	ResponseFormat    string               `json:"response_format,omitempty"`
	Stream            *bool                `json:"stream,omitempty"`
	Background        string               `json:"background,omitempty"`
	InputFidelity     string               `json:"input_fidelity,omitempty"`
	Moderation        string               `json:"moderation,omitempty"`
	OutputFormat      string               `json:"output_format,omitempty"`
	OutputCompression *int                 `json:"output_compression,omitempty"`
	User              string               `json:"user,omitempty"`
}

type consoleImageSource struct {
	ImageURL string `json:"image_url"`
}

type consoleImageEffectivePolicy struct {
	StreamMode string
}

func (s *Console) consoleImagePolicyForModel(ctx context.Context, subject *coreidentity.Subject, modelCode string) (consoleImageEffectivePolicy, error) {
	policy := defaultConsoleImagePolicy()
	if s == nil || s.postgres == nil {
		return policy, nil
	}
	groups, err := s.grantChecker.AccessibleGroupIDsForSubject(ctx, subject)
	if err != nil {
		return policy, err
	}
	if len(groups) == 0 {
		return policy, nil
	}
	var raw []byte
	err = s.postgres.QueryRow(ctx, `
		SELECT um.config_json
		FROM ai_group_targets gt
		JOIN ai_groups g
		  ON g.id = gt.group_id AND g.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.model_code = $1
		 AND um.capability_type = 'image'
		 AND um.status = 'active'
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		WHERE gt.group_id = ANY($2::uuid[])
		  AND gt.status = 'active'
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		ORDER BY array_position($2::uuid[], gt.group_id), gt.id ASC
		LIMIT 1
	`, modelCode, groups).Scan(&raw)
	if err == pgx.ErrNoRows {
		return policy, nil
	}
	if err != nil {
		return policy, err
	}
	return parseConsoleImagePolicy(raw), nil
}

func defaultConsoleImagePolicy() consoleImageEffectivePolicy {
	return consoleImageEffectivePolicy{
		StreamMode: domain.ImageStreamModeForceSync,
	}
}

func parseConsoleImagePolicy(raw []byte) consoleImageEffectivePolicy {
	policy := defaultConsoleImagePolicy()
	if len(raw) == 0 {
		return policy
	}
	var cfg struct {
		ImageGeneration struct {
			StreamMode string `json:"stream_mode"`
		} `json:"image_generation"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return policy
	}
	switch strings.TrimSpace(cfg.ImageGeneration.StreamMode) {
	case domain.ImageStreamModeAuto, domain.ImageStreamModeForceStream, domain.ImageStreamModeForceSync:
		policy.StreamMode = strings.TrimSpace(cfg.ImageGeneration.StreamMode)
	}
	return policy
}

func applyConsoleImagePolicy(req *consoleImageGenerateRequest, policy consoleImageEffectivePolicy, autoStream bool) {
	if req == nil {
		return
	}
	// The console is an API client that always consumes image URLs. The binding
	// still controls what is requested from the upstream; the serving relay
	// converts that upstream response back to URL when necessary.
	req.ResponseFormat = domain.ImageResponseFormatURL
	if req.Stream == nil {
		enabled := autoStream
		switch policy.StreamMode {
		case domain.ImageStreamModeForceStream:
			enabled = true
		case domain.ImageStreamModeForceSync:
			enabled = false
		}
		req.Stream = &enabled
	}
}

func consoleImageRequestStreamEnabled(req consoleImageGenerateRequest) bool {
	return req.Stream != nil && *req.Stream
}

func consoleImageRequestFromTaskInput(input consoleImageTaskInputPayload, fallbackModel string) consoleImageGenerateRequest {
	req := consoleImageGenerateRequest{
		Operation:         strings.TrimSpace(input.Operation),
		ModelCode:         strings.TrimSpace(input.Model),
		GroupID:           strings.TrimSpace(input.GroupID),
		Prompt:            strings.TrimSpace(input.Prompt),
		N:                 input.N,
		Images:            input.Images,
		Mask:              input.Mask,
		Size:              strings.TrimSpace(input.Size),
		ResponseFormat:    strings.TrimSpace(input.ResponseFormat),
		Background:        strings.TrimSpace(input.Background),
		InputFidelity:     strings.TrimSpace(input.InputFidelity),
		Moderation:        strings.TrimSpace(input.Moderation),
		OutputFormat:      strings.TrimSpace(input.OutputFormat),
		OutputCompression: input.OutputCompression,
		User:              strings.TrimSpace(input.User),
	}
	if req.ModelCode == "" {
		req.ModelCode = strings.TrimSpace(fallbackModel)
	}
	return req
}

func (s *Console) ensureConsoleImageModelGranted(ctx context.Context, subject *coreidentity.Subject, modelCode string) error {
	groups, err := s.grantChecker.AccessibleGroupIDsForSubject(ctx, subject)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return domain.ErrForbidden
	}
	var groupID string
	err = s.postgres.QueryRow(ctx, `
		SELECT g.id::text
		FROM ai_group_targets gt
		JOIN ai_groups g
		  ON g.id = gt.group_id AND g.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.status = 'active'
		 AND um.capability_type = 'image'
		 AND um.model_code = $2
		JOIN ai_price_book_entries e
		  ON e.price_book_id = g.retail_price_book_id
		 AND e.model_code = um.model_code
		 AND e.capability_type = um.capability_type
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		WHERE gt.group_id = ANY($1::uuid[])
		  AND gt.status = 'active'
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		ORDER BY array_position($1::uuid[], g.id)
		LIMIT 1
	`, groups, modelCode).Scan(&groupID)
	if err == pgx.ErrNoRows {
		return domain.ErrForbidden
	}
	if err != nil {
		return err
	}
	ok, err := s.routeInspector.ModelSupportsClientProtocolInGroups(ctx, modelCode, domain.CapabilityImage, []string{groupID}, domain.ProtocolOpenAIImages, false, true)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden
	}
	return nil
}

func buildConsoleImageBody(req consoleImageGenerateRequest, modelCode, prompt string) ([]byte, error) {
	body := map[string]any{}
	body["model"] = modelCode
	body["prompt"] = prompt
	if req.N > domain.DefaultImageOutputCount {
		body["n"] = req.N
	}
	if req.Size != "" {
		body["size"] = req.Size
	}
	if req.ResponseFormat != "" {
		body["response_format"] = req.ResponseFormat
	} else {
		body["response_format"] = defaultConsoleImageResponseFormat
	}
	if req.Stream != nil {
		body["stream"] = *req.Stream
	}
	if req.Background != "" {
		body["background"] = req.Background
	}
	if req.OutputFormat != "" {
		body["output_format"] = req.OutputFormat
	}
	return json.Marshal(body)
}

func decodeConsoleImageEditRequest(req consoleImageGenerateRequest) (imageedit.Request, error) {
	stream := req.Stream != nil && *req.Stream
	payload := map[string]any{
		"images":          req.Images,
		"prompt":          strings.TrimSpace(req.Prompt),
		"size":            strings.TrimSpace(req.Size),
		"background":      strings.TrimSpace(req.Background),
		"input_fidelity":  strings.TrimSpace(req.InputFidelity),
		"moderation":      strings.TrimSpace(req.Moderation),
		"output_format":   strings.TrimSpace(req.OutputFormat),
		"stream":          stream,
		"user":            strings.TrimSpace(req.User),
		"response_format": effectiveConsoleImageResponseFormat(req.ResponseFormat),
	}
	if req.N > 0 {
		payload["n"] = req.N
	}
	if req.Mask != nil {
		payload["mask"] = req.Mask
	}
	if req.OutputCompression != nil {
		payload["output_compression"] = *req.OutputCompression
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return imageedit.Request{}, err
	}
	return imageedit.Decode(body, imageedit.TransportJSON)
}

func buildConsoleImageTaskInput(operation string, req consoleImageGenerateRequest, hasSourceImage, hasMask bool) []byte {
	payload := map[string]any{
		"operation":          operation,
		"model":              strings.TrimSpace(req.ModelCode),
		"prompt":             strings.TrimSpace(req.Prompt),
		"n":                  req.N,
		"group_id":           strings.TrimSpace(req.GroupID),
		"size":               req.Size,
		"response_format":    effectiveConsoleImageResponseFormat(req.ResponseFormat),
		"background":         req.Background,
		"input_fidelity":     req.InputFidelity,
		"moderation":         req.Moderation,
		"output_format":      req.OutputFormat,
		"output_compression": req.OutputCompression,
		"user":               req.User,
		"has_source_image":   hasSourceImage,
		"has_mask":           hasMask,
	}
	if len(req.Images) > 0 {
		payload["images"] = req.Images
	}
	if req.Mask != nil {
		payload["mask"] = req.Mask
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func buildConsoleImageEditTaskInput(req consoleImageGenerateRequest, hasMask bool, editRequest imageedit.Request) ([]byte, error) {
	raw := buildConsoleImageTaskInput("edit", req, true, hasMask)
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode image task input: %w", err)
	}
	editRequest.Model = ""
	editRequest.Prompt = ""
	canonicalBody, err := imageedit.CanonicalJSON(editRequest)
	if err != nil {
		return nil, fmt.Errorf("encode persisted image edit request: %w", err)
	}
	payload["edit_request"] = json.RawMessage(canonicalBody)
	persisted, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode image task input: %w", err)
	}
	return persisted, nil
}

func decodePersistedConsoleImageEditRequest(inputPayload []byte) (imageedit.Request, error) {
	var input consoleImageTaskInputPayload
	if err := json.Unmarshal(inputPayload, &input); err != nil {
		return imageedit.Request{}, fmt.Errorf("decode task input: %w", err)
	}
	if len(bytes.TrimSpace(input.EditRequest)) == 0 {
		return imageedit.Request{}, errors.New("persisted image edit request is missing")
	}
	req, err := imageedit.Decode(input.EditRequest, imageedit.TransportJSON)
	if err != nil {
		return imageedit.Request{}, fmt.Errorf("decode persisted image edit request: %w", err)
	}
	return req, nil
}

const defaultConsoleImageResponseFormat = domain.ImageResponseFormatURL

func effectiveConsoleImageResponseFormat(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return domain.ImageResponseFormatURL
	}
	return value
}
