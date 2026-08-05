// Package billingledger coordinates PAYG credit leases, durable request
// admission, and asynchronous settlement. PostgreSQL is the source of truth;
// the remote URM dependency is accessed through LeasePort.
package billingledger

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/urm"
)

var (
	ErrInsufficientBalance   = errors.New("billing ledger: insufficient balance")
	ErrDependencyUnavailable = errors.New("billing ledger: dependency unavailable")
	ErrAdmissionConflict     = errors.New("billing ledger: admission conflict")
	ErrProtocolViolation     = errors.New("billing ledger: protocol violation")
)

type LeasePort interface {
	AcquireCreditLease(context.Context, urm.AcquireCreditLeaseRequest) (*urm.CreditLeaseResponse, error)
	RenewCreditLease(context.Context, string, urm.RenewCreditLeaseRequest) (*urm.CreditLeaseResponse, error)
	SettleCreditLease(context.Context, string, urm.SettleCreditLeaseRequest) (*urm.CreditLeaseResponse, error)
	GetCreditLease(context.Context, string) (*urm.CreditLeaseResponse, error)
}

type BillingSnapshot struct {
	OpeningWindows         int64
	ActiveWindows          int64
	DrainingWindows        int64
	ReconcilingWindows     int64
	SettlementPending      int64
	PendingOutbox          int64
	OldestOutboxAgeSeconds float64
	ReconcilingAdmissions  int64
}

// Observer is deliberately small and value-only so the financial module does
// not depend on a particular metrics backend.
type Observer interface {
	BillingLeaseOperation(operation, result string)
	BillingSettlementDispatch(result string)
	SetBillingSnapshot(BillingSnapshot)
}

type Config struct {
	RequestedTenantMicro int64
	RequestedUserMicro   int64
	LeaseTTL             time.Duration
	LeaseGrace           time.Duration
	WindowMaxAge         time.Duration
	RenewLead            time.Duration
	AdmissionHeadroom    time.Duration
	WorkerInterval       time.Duration
	DispatchLease        time.Duration
	PickLimit            int
}

func (c Config) withDefaults() Config {
	if c.RequestedTenantMicro <= 0 {
		c.RequestedTenantMicro = 300_000
	}
	if c.RequestedUserMicro <= 0 {
		c.RequestedUserMicro = 300_000
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 5 * time.Minute
	}
	if c.LeaseGrace <= 0 {
		c.LeaseGrace = 15 * time.Minute
	}
	if c.WindowMaxAge <= 0 {
		c.WindowMaxAge = 3 * time.Minute
	}
	if c.RenewLead <= 0 {
		c.RenewLead = 90 * time.Second
	}
	if c.AdmissionHeadroom <= 0 {
		c.AdmissionHeadroom = 30 * time.Second
	}
	if c.WorkerInterval <= 0 {
		c.WorkerInterval = 5 * time.Second
	}
	if c.DispatchLease <= 0 {
		c.DispatchLease = 30 * time.Second
	}
	if c.PickLimit <= 0 {
		c.PickLimit = 32
	}
	return c
}

type Intent struct {
	RequestID  string
	OwnerType  string
	TenantID   string
	UserID     string
	WantTenant bool
	WantUser   bool
	RequestTTL time.Duration
}

type Admission struct {
	RequestID string
	WindowID  string
	LeaseID   string
}

type Completion struct {
	RequestID       string
	ExpectedLeaseID string
	TenantMicro     int64
	UserMicro       int64
	Source          string
	Note            string
}

type Coordinator struct {
	pool     *pgxpool.Pool
	port     LeasePort
	config   Config
	logger   *zap.Logger
	now      func() time.Time
	trigger  chan struct{}
	observer Observer
}

func New(pool *pgxpool.Pool, port LeasePort, cfg Config, logger *zap.Logger, observers ...Observer) *Coordinator {
	if logger == nil {
		logger = zap.NewNop()
	}
	var observer Observer
	if len(observers) > 0 {
		observer = observers[0]
	}
	return &Coordinator{
		pool: pool, port: port, config: cfg.withDefaults(), logger: logger,
		now: time.Now, trigger: make(chan struct{}, 1), observer: observer,
	}
}

func (c *Coordinator) observeLeaseOperation(operation, result string) {
	if c != nil && c.observer != nil {
		c.observer.BillingLeaseOperation(operation, result)
	}
}

func (c *Coordinator) observeSettlement(result string) {
	if c != nil && c.observer != nil {
		c.observer.BillingSettlementDispatch(result)
	}
}

func (c *Coordinator) Trigger() {
	if c == nil {
		return
	}
	select {
	case c.trigger <- struct{}{}:
	default:
	}
}

func classifyPortError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrInsufficientBalance) || errors.Is(err, ErrAdmissionConflict) ||
		errors.Is(err, ErrProtocolViolation) || errors.Is(err, ErrDependencyUnavailable) {
		return err
	}
	var apiErr *urm.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == http.StatusPaymentRequired {
			return fmt.Errorf("%w: %w", ErrInsufficientBalance, err)
		}
		if apiErr.Status == http.StatusConflict {
			return fmt.Errorf("%w: %w", ErrAdmissionConflict, err)
		}
	}
	return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}
