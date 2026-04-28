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
}

func (s *Server) freezeChatSettlement(ctx context.Context, auth RuntimeAuth, requestID string, platformAmount int64, userAmount int64) (*settlementReservation, error) {
	if s.urmClient == nil || (platformAmount <= 0 && userAmount <= 0) {
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
		TenantAmount: platformAmount,
		UserAmount:   userAmount,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.TransactionID == "" {
		return nil, errors.New("urm freeze returned empty transaction id")
	}

	return &settlementReservation{
		transactionID: resp.TransactionID,
		tenantAmount:  platformAmount,
		userAmount:    userAmount,
	}, nil
}

func (s *Server) confirmChatSettlement(ctx context.Context, reservation *settlementReservation, costs chatCosts) string {
	if s.urmClient == nil || reservation == nil || reservation.transactionID == "" {
		return "not_billed"
	}
	if _, err := s.urmClient.Confirm(ctx, urm.ConfirmRequest{
		TransactionID:      reservation.transactionID,
		ActualTenantAmount: costs.PlatformCost,
		ActualUserAmount:   costs.UserCost,
	}); err != nil {
		s.logger.Error("confirm urm settlement failed", "error", err, "transaction_id", reservation.transactionID)
		return "confirm_failed"
	}
	return "confirmed"
}

func (s *Server) cancelChatSettlement(ctx context.Context, reservation *settlementReservation) string {
	if s.urmClient == nil || reservation == nil || reservation.transactionID == "" {
		return "not_billed"
	}
	if err := s.urmClient.Cancel(ctx, reservation.transactionID); err != nil {
		s.logger.Error("cancel urm settlement failed", "error", err, "transaction_id", reservation.transactionID)
		return "cancel_failed"
	}
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

func estimatedSettlementCosts(raw map[string]json.RawMessage, model callableModel, price modelPrice, auth RuntimeAuth) chatCosts {
	outputTokens := requestedOutputTokens(raw, model.DefaultMaxOutputTokens)
	tenantSaleCost := tokenCost(outputTokens, price.OutputPricePer1m)
	platform := tenantSaleCost
	user := int64(0)
	if auth.APIKey.OwnerType == "user" {
		user = tenantSaleCost
	}
	return chatCosts{
		PlatformCost:    platform,
		UserCost:        user,
		APIKeyQuotaCost: tenantSaleCost,
	}
}
