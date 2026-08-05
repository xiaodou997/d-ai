package clienttransport

import (
	"context"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/serving"
)

// Adapter lets the client runtime reuse the existing production HTTP transport
// without coupling the clientruntime module to serving's transport types.
type Adapter struct {
	transport serving.Transporter
}

func New(transport serving.Transporter) *Adapter {
	return &Adapter{transport: transport}
}

func (a *Adapter) Do(ctx context.Context, req *clientruntime.WireRequest) (*clientruntime.WireResponse, error) {
	response, err := a.transport.Do(ctx, &serving.UpstreamRequest{
		Method:   req.Method,
		URL:      req.URL,
		Headers:  req.Headers,
		Body:     req.Body,
		Protocol: req.Protocol,
	})
	if err != nil {
		return nil, err
	}
	return &clientruntime.WireResponse{
		StatusCode: response.StatusCode,
		Headers:    response.Headers,
		Body:       response.Body,
	}, nil
}
