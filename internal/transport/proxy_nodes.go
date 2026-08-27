package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	proxypkg "xiaodou/dai/internal/ai/proxy"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/httpx"
)

type proxyNodesOutput struct{ Body []proxypkg.Node }
type proxyNodeOutput struct{ Body proxypkg.Node }
type proxyNodePathInput struct {
	ID string `path:"id"`
}
type proxyNodeBody struct {
	Name      string `json:"name" maxLength:"100"`
	ProxyType string `json:"proxyType" enum:"http,socks5"`
	Endpoint  string `json:"endpoint" maxLength:"500"`
	Username  string `json:"username,omitempty" required:"false"`
	Password  string `json:"password,omitempty" required:"false"`
	Weight    int    `json:"weight,omitempty" required:"false"`
	Status    string `json:"status,omitempty" enum:"active,disabled" required:"false"`
}
type proxyNodeInput struct{ Body proxyNodeBody }
type proxyNodeUpdateInput struct {
	ID   string `path:"id"`
	Body proxyNodeBody
}

func registerProxyNodes(api huma.API, d proxyNodesModule) {
	admin := huma.Middlewares{userAuth(api, d.auth.JWT, d.auth.Blacklist), requireCapability(api, auth.CapabilityPlatformAdmin)}
	huma.Register(api, huma.Operation{OperationID: "admin-list-proxy-nodes", Method: http.MethodGet, Path: "/api/v1/admin/proxy-nodes", Summary: "代理出口节点", Tags: []string{"proxy-nodes"}, Middlewares: admin}, func(ctx context.Context, _ *struct{}) (*proxyNodesOutput, error) {
		items, err := d.service.List(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		return &proxyNodesOutput{Body: items}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "admin-create-proxy-node", Method: http.MethodPost, Path: "/api/v1/admin/proxy-nodes", Summary: "创建代理出口节点", Tags: []string{"proxy-nodes"}, Middlewares: admin, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *proxyNodeInput) (*proxyNodeOutput, error) {
		item, err := d.service.Upsert(ctx, "", proxyInput(in.Body), actorID(ctx))
		if err != nil {
			return nil, proxyNodeHTTPError(err)
		}
		return &proxyNodeOutput{Body: item}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "admin-update-proxy-node", Method: http.MethodPut, Path: "/api/v1/admin/proxy-nodes/{id}", Summary: "更新代理出口节点", Tags: []string{"proxy-nodes"}, Middlewares: admin}, func(ctx context.Context, in *proxyNodeUpdateInput) (*proxyNodeOutput, error) {
		item, err := d.service.Upsert(ctx, in.ID, proxyInput(in.Body), actorID(ctx))
		if errors.Is(err, proxypkg.ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		if err != nil {
			return nil, proxyNodeHTTPError(err)
		}
		return &proxyNodeOutput{Body: item}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "admin-delete-proxy-node", Method: http.MethodDelete, Path: "/api/v1/admin/proxy-nodes/{id}", Summary: "删除代理出口节点", Tags: []string{"proxy-nodes"}, Middlewares: admin}, func(ctx context.Context, in *proxyNodePathInput) (*messageOutput, error) {
		if err := d.service.Delete(ctx, in.ID); errors.Is(err, proxypkg.ErrNotFound) {
			return nil, httpx.ErrNotFound
		} else if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		out := &messageOutput{}
		out.Body.Message = "代理节点已删除"
		return out, nil
	})
}

func proxyInput(body proxyNodeBody) proxypkg.UpsertInput {
	return proxypkg.UpsertInput{Name: body.Name, ProxyType: body.ProxyType, Endpoint: body.Endpoint, Username: body.Username, Password: body.Password, Weight: body.Weight, Status: body.Status}
}

func actorID(ctx context.Context) string {
	if claims := userClaimsFromCtx(ctx); claims != nil {
		return claims.UserID
	}
	return ""
}

func proxyNodeHTTPError(err error) error {
	if errors.Is(err, proxypkg.ErrInvalidInput) || errors.Is(err, proxypkg.ErrInvalidEndpoint) {
		return httpx.ErrBadRequest.WithCause(err)
	}
	return httpx.ErrInternal.WithCause(err)
}
