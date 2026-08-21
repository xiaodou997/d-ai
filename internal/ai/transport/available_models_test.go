package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestListAvailableModelsForScopeAggregatesPriceRanges(t *testing.T) {
	reader := &modelCatalogReaderTransportStub{
		available: []domain.RoutedModelPrice{
			{
				ModelCode: "image-model", CapabilityType: "image",
				ImageDefaultPrice: 0.08,
				ImagePrices:       []domain.ResolutionUSDPrice{{Resolution: "1024x1024", Price: 0.09}},
			},
			{
				ModelCode: "chat-model", CapabilityType: "chat",
				TokenPriceTiers: []domain.TokenPriceTier{
					{InputPerToken: 0.000002, OutputPerToken: 0.000004},
					{InputPerToken: 0.000003, OutputPerToken: 0.000006},
				},
			},
			{
				ModelCode: "image-model", CapabilityType: "image",
				ImageDefaultPrice: 0.04,
				ImagePrices: []domain.ResolutionUSDPrice{
					{Resolution: "1024x1024", Price: 0.05},
					{Resolution: "2048x2048", Price: 0.12},
				},
			},
		},
	}

	items, err := listAvailableModelsForScope(context.Background(), reader, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("listAvailableModelsForScope: %v", err)
	}
	if reader.scope != (domain.ModelCatalogScope{TenantID: "tenant-1", UserID: "user-1"}) {
		t.Fatalf("scope = %+v", reader.scope)
	}
	if len(items) != 2 || items[0].ModelCode != "chat-model" || items[1].ModelCode != "image-model" {
		t.Fatalf("items = %+v", items)
	}
	if !items[0].HasContextTiers || items[0].InputPer1MUSDMin != 2 || items[0].InputPer1MUSDMax != 3 || items[0].OutputPer1MUSDMin != 4 || items[0].OutputPer1MUSDMax != 6 {
		t.Fatalf("chat price range = %+v", items[0])
	}
	image := items[1]
	if image.ImageDefaultPriceMin != 0.04 || image.ImageDefaultPriceMax != 0.08 || len(image.ImagePrices) != 2 {
		t.Fatalf("image price range = %+v", image)
	}
	if image.ImagePrices[0].Resolution != "1024x1024" || image.ImagePrices[0].PriceUSDMin != 0.05 || image.ImagePrices[0].PriceUSDMax != 0.09 {
		t.Fatalf("1024 range = %+v", image.ImagePrices[0])
	}
}

type modelCatalogReaderTransportStub struct {
	available []domain.RoutedModelPrice
	scope     domain.ModelCatalogScope
}

func (s *modelCatalogReaderTransportStub) ListAvailableModelPrices(_ context.Context, scope domain.ModelCatalogScope) ([]domain.RoutedModelPrice, error) {
	s.scope = scope
	return s.available, nil
}

func (*modelCatalogReaderTransportStub) ListRoutedGroupPrices(context.Context, string) ([]domain.RoutedModelPrice, error) {
	return nil, nil
}

func (*modelCatalogReaderTransportStub) ListTenantUpstreamResources(context.Context, string) ([]domain.TenantUpstreamResource, error) {
	return nil, nil
}
