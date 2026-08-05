package billing

// EventRepository 消费事件仓储接口
type EventRepository interface {
	Save(event *BillingEvent) error
	FindByEventID(eventID string) (*BillingEvent, error)
	FindByIdempotencyKey(key string) (*BillingEvent, error)
	FindByUserID(userID string, limit int) ([]*BillingEvent, error)
	CountToday() (int64, error)
	CountTodaySuccess() (int64, error)
	CountActivePreAuth() (int64, error)
	FindStuckPreAuth(timeoutMinutes int) ([]*BillingEvent, error)
	FindReleasedInHours(hours, limit int) ([]*BillingEvent, error)
}
