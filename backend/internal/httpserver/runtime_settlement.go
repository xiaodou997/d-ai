package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"uni-ai-api/backend/internal/urm"
)

type settlementReservation struct {
	transactionID string
	tenantAmount  int64
	userAmount    int64
	ownerType     string
}

func (s *Server) freezeChatSettlement(ctx context.Context, auth RuntimeAuth, requestID string, tenantAmount int64, userAmount int64) (*settlementReservation, error) {
	if s.urmClient == nil || (tenantAmount <= 0 && userAmount <= 0) {
		return nil, nil
	}

	customerID := ""
	if auth.APIKey.OwnerType == "user" && auth.APIKey.UserID.Valid {
		customerID = auth.APIKey.UserID.String
	}
	resp, err := s.urmClient.Freeze(ctx, urm.FreezeRequest{
		RequestID:    requestID,
		TenantID:     auth.APIKey.TenantID,
		CustomerID:   customerID,
		Description:  "uni-ai-api chat completion",
		TenantAmount: tenantAmount,
		UserAmount:   userAmount,
	})
	if err != nil {
		s.logger.Warn("gateway settlement freeze failed",
			"error", err,
			"error_code", errorCodeSettlementFailed,
			"request_id", requestID,
			"tenant_id", auth.APIKey.TenantID,
			"owner_type", auth.APIKey.OwnerType,
			"tenant_amount", tenantAmount,
			"user_amount", userAmount,
		)
		return nil, err
	}
	if resp == nil || resp.TransactionID == "" {
		return nil, errors.New("urm freeze returned empty transaction id")
	}
	s.logger.Debug("gateway settlement frozen",
		"request_id", requestID,
		"tenant_id", auth.APIKey.TenantID,
		"owner_type", auth.APIKey.OwnerType,
		"transaction_id", resp.TransactionID,
		"tenant_amount", tenantAmount,
		"user_amount", userAmount,
	)

	return &settlementReservation{
		transactionID: resp.TransactionID,
		tenantAmount:  tenantAmount,
		userAmount:    userAmount,
		ownerType:     auth.APIKey.OwnerType,
	}, nil
}

func (s *Server) confirmChatSettlement(ctx context.Context, reservation *settlementReservation, costs chatCosts) string {
	if s.urmClient == nil || reservation == nil || reservation.transactionID == "" {
		return "not_billed"
	}
	if _, err := s.urmClient.Confirm(ctx, urm.ConfirmRequest{
		TransactionID:      reservation.transactionID,
		ActualTenantAmount: costs.TenantCost,
		ActualUserAmount:   costs.UserCost,
	}); err != nil {
		s.logger.Error("gateway settlement confirm failed", "error", err, "error_code", errorCodeSettlementFailed, "transaction_id", reservation.transactionID)
		return "confirm_failed"
	}
	s.logger.Debug("gateway settlement confirmed",
		"transaction_id", reservation.transactionID,
		"estimated_tenant_amount", reservation.tenantAmount,
		"estimated_user_amount", reservation.userAmount,
		"actual_tenant_amount", costs.TenantCost,
		"actual_user_amount", costs.UserCost,
	)
	return "confirmed"
}

func (s *Server) cancelChatSettlement(ctx context.Context, reservation *settlementReservation) string {
	if s.urmClient == nil || reservation == nil || reservation.transactionID == "" {
		return "not_billed"
	}
	if err := s.urmClient.Cancel(ctx, reservation.transactionID); err != nil {
		s.logger.Error("gateway settlement cancel failed", "error", err, "error_code", errorCodeSettlementFailed, "transaction_id", reservation.transactionID)
		return "cancel_failed"
	}
	s.logger.Debug("gateway settlement canceled", "transaction_id", reservation.transactionID)
	return "canceled"
}

func isURMInsufficientBalance(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "insufficient") ||
		strings.Contains(message, "余额不足") ||
		strings.Contains(message, "not enough")
}

func estimatedSettlementCosts(raw map[string]json.RawMessage, model callableModel, tenantCostPrice modelPrice, userSalePrice modelPrice, auth RuntimeAuth) chatCosts {
	outputTokens := requestedOutputTokens(raw, model.DefaultMaxOutputTokens)
	// Tenant cost (what tenant pays to platform)
	tenantCost := tokenCost(outputTokens, tenantCostPrice.OutputPricePer1m)
	// User sale price (what user pays to tenant)
	userCost := int64(0)
	if auth.APIKey.OwnerType == "user" {
		userCost = tokenCost(outputTokens, userSalePrice.OutputPricePer1m)
	}
	// API Key quota cost uses the sale price (or tenant cost for anonymous)
	quotaCost := tenantCost
	if auth.APIKey.OwnerType == "user" {
		quotaCost = userCost
	}
	return chatCosts{
		TenantCost:      tenantCost,
		UserCost:        userCost,
		APIKeyQuotaCost: quotaCost,
	}
}
