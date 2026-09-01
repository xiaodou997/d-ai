package tenant

import (
	"context"
	"errors"
	"sync"
	"time"

	tenantports "xiaodou/dai/internal/tenant/ports"
)

const deletionGracePeriod = 24 * time.Hour

type DeletionService struct {
	store tenantports.TenantDeletionStore
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
}

var _ tenantports.TenantDeletionService = (*DeletionService)(nil)

func NewDeletionService(store tenantports.TenantDeletionStore) *DeletionService {
	return &DeletionService{store: store}
}

func (s *DeletionService) Request(ctx context.Context, tenantID, requestedBy string) (tenantports.TenantDeletionJob, error) {
	if s == nil || s.store == nil {
		return tenantports.TenantDeletionJob{}, errors.New("tenant deletion service is not configured")
	}
	return s.store.RequestDeletion(ctx, tenantID, requestedBy, time.Now().UTC().Add(deletionGracePeriod))
}
func (s *DeletionService) Cancel(ctx context.Context, tenantID string) (bool, error) {
	if s == nil || s.store == nil {
		return false, errors.New("tenant deletion service is not configured")
	}
	return s.store.CancelDeletion(ctx, tenantID)
}
func (s *DeletionService) Get(ctx context.Context, tenantID string) (tenantports.TenantDeletionJob, error) {
	if s == nil || s.store == nil {
		return tenantports.TenantDeletionJob{}, errors.New("tenant deletion service is not configured")
	}
	return s.store.GetDeletion(ctx, tenantID)
}
func (s *DeletionService) Start(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	s.once.Do(func() {
		s.stop, s.done = make(chan struct{}), make(chan struct{})
		go func() {
			defer close(s.done)
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-s.stop:
					return
				case <-ticker.C:
					runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					s.runDue(runCtx)
					cancel()
				}
			}
		}()
	})
}
func (s *DeletionService) Stop(ctx context.Context) {
	if s == nil || s.stop == nil {
		return
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	select {
	case <-s.done:
	case <-ctx.Done():
	}
}
func (s *DeletionService) runDue(ctx context.Context) {
	// The store claims and executes one due job atomically; repeat until empty.
	for i := 0; i < 100; i++ {
		job, err := s.store.GetDueDeletion(ctx)
		if err != nil || job.JobID == "" {
			return
		}
		_ = s.store.RunDeletion(ctx, job.JobID, job.TenantID)
	}
}
