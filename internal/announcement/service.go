package announcement

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"xiaodou/dai/internal/auth"
)

var (
	ErrForbidden         = errors.New("announcement operation forbidden")
	ErrInvalidTitle      = errors.New("announcement title is invalid")
	ErrInvalidContent    = errors.New("announcement content is invalid")
	ErrInvalidAudience   = errors.New("announcement audience is invalid")
	ErrInvalidSchedule   = errors.New("announcement schedule is invalid")
	ErrInvalidMetadata   = errors.New("announcement metadata is invalid")
	ErrInvalidTransition = errors.New("announcement state transition is invalid")
	ErrUnavailable       = errors.New("announcement repository capability unavailable")
	ErrNotFound          = errors.New("announcement not found")
)

type draftRepository interface {
	CreateDraft(ctx context.Context, item Announcement) (Announcement, error)
}

type managedReader interface {
	GetManaged(ctx context.Context, actor Actor, id string) (Announcement, error)
}

type draftUpdater interface {
	UpdateDraft(ctx context.Context, actor Actor, item Announcement) (Announcement, error)
}

type publisher interface {
	Publish(ctx context.Context, actor Actor, id string, now time.Time) (Announcement, error)
}

type inboxRepository interface {
	ListInbox(ctx context.Context, principal Principal, query InboxQuery) (InboxPage, error)
	GetVisible(ctx context.Context, principal Principal, id string) (InboxItem, error)
	MarkRead(ctx context.Context, principal Principal, id string, now time.Time) error
}

type archiver interface {
	Archive(ctx context.Context, actor Actor, id string, now time.Time) (Announcement, error)
}

type statsRepository interface {
	GetStats(ctx context.Context, actor Actor, id string) (Stats, error)
}

type deleter interface {
	Delete(ctx context.Context, actor Actor, id string) error
}

type managementRepository interface {
	ListManaged(ctx context.Context, actor Actor, query ManageQuery) (ManagedPage, error)
	ListRecipients(ctx context.Context, actor Actor, id string, query RecipientQuery) (RecipientPage, error)
}

type Service struct {
	repo      draftRepository
	reader    managedReader
	updater   draftUpdater
	publisher publisher
	inbox     inboxRepository
	archiver  archiver
	stats     statsRepository
	deleter   deleter
	manager   managementRepository
}

func NewService(repo draftRepository) *Service {
	service := &Service{repo: repo}
	service.reader, _ = repo.(managedReader)
	service.updater, _ = repo.(draftUpdater)
	service.publisher, _ = repo.(publisher)
	service.inbox, _ = repo.(inboxRepository)
	service.archiver, _ = repo.(archiver)
	service.stats, _ = repo.(statsRepository)
	service.deleter, _ = repo.(deleter)
	service.manager, _ = repo.(managementRepository)
	return service
}

func (s *Service) GetManaged(ctx context.Context, actor Actor, id string) (Announcement, error) {
	if s.reader == nil {
		return Announcement{}, ErrUnavailable
	}
	return s.reader.GetManaged(ctx, actor, id)
}

func (s *Service) Delete(ctx context.Context, actor Actor, id string) error {
	if s.deleter == nil {
		return ErrUnavailable
	}
	return s.deleter.Delete(ctx, actor, id)
}

// DeleteDraft is kept as a compatibility alias for callers that still use the
// old draft-only name. Deletion is now available for every announcement state.
func (s *Service) DeleteDraft(ctx context.Context, actor Actor, id string) error {
	return s.Delete(ctx, actor, id)
}

func (s *Service) ListManaged(ctx context.Context, actor Actor, query ManageQuery) (ManagedPage, error) {
	if s.manager == nil {
		return ManagedPage{}, ErrUnavailable
	}
	if !actor.Has(auth.CapabilityPlatformAdmin) && !actor.Has(auth.CapabilityTenantSelf) {
		return ManagedPage{}, ErrForbidden
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Size < 1 || query.Size > 100 {
		query.Size = 20
	}
	query.Search = strings.TrimSpace(query.Search)
	return s.manager.ListManaged(ctx, actor, query)
}

func (s *Service) ListRecipients(ctx context.Context, actor Actor, id string, query RecipientQuery) (RecipientPage, error) {
	if s.manager == nil {
		return RecipientPage{}, ErrUnavailable
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Size < 1 || query.Size > 100 {
		query.Size = 20
	}
	query.Search = strings.TrimSpace(query.Search)
	return s.manager.ListRecipients(ctx, actor, id, query)
}

func (s *Service) Archive(ctx context.Context, actor Actor, id string) (Announcement, error) {
	if s.archiver == nil {
		return Announcement{}, ErrUnavailable
	}
	return s.archiver.Archive(ctx, actor, id, time.Now().UTC())
}

func (s *Service) GetStats(ctx context.Context, actor Actor, id string) (Stats, error) {
	if s.stats == nil {
		return Stats{}, ErrUnavailable
	}
	return s.stats.GetStats(ctx, actor, id)
}

func (s *Service) ListInbox(ctx context.Context, principal Principal, query InboxQuery) (InboxPage, error) {
	if s.inbox == nil {
		return InboxPage{}, ErrUnavailable
	}
	if err := validatePrincipal(principal); err != nil {
		return InboxPage{}, err
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Size < 1 || query.Size > 100 {
		query.Size = 20
	}
	if query.DisplayMode != "" && query.DisplayMode != DisplayInbox && query.DisplayMode != DisplayPopup {
		return InboxPage{}, ErrInvalidMetadata
	}
	return s.inbox.ListInbox(ctx, principal, query)
}

func (s *Service) MarkRead(ctx context.Context, principal Principal, id string) error {
	if s.inbox == nil {
		return ErrUnavailable
	}
	if err := validatePrincipal(principal); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}
	return s.inbox.MarkRead(ctx, principal, id, time.Now().UTC())
}

func (s *Service) GetVisible(ctx context.Context, principal Principal, id string) (InboxItem, error) {
	if s.inbox == nil {
		return InboxItem{}, ErrUnavailable
	}
	if err := validatePrincipal(principal); err != nil {
		return InboxItem{}, err
	}
	return s.inbox.GetVisible(ctx, principal, id)
}

func validatePrincipal(principal Principal) error {
	if strings.TrimSpace(string(principal.UserID)) == "" {
		return ErrForbidden
	}
	actor := principal.AuthorizationActor()
	if actor.Has(auth.CapabilityPlatformAdmin) || actor.Has(auth.CapabilityTenantSelf) || actor.Has(auth.CapabilityCustomerSelf) {
		return nil
	}
	return ErrForbidden
}

func (s *Service) CreateDraft(ctx context.Context, actor Actor, input DraftInput) (Announcement, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" || len([]rune(title)) > 200 {
		return Announcement{}, ErrInvalidTitle
	}
	content := strings.TrimSpace(input.ContentMarkdown)
	if content == "" || len([]rune(content)) > 50000 {
		return Announcement{}, ErrInvalidContent
	}
	if input.StartsAt != nil && input.EndsAt != nil && !input.StartsAt.Before(*input.EndsAt) {
		return Announcement{}, ErrInvalidSchedule
	}
	category, severity, displayMode, err := normalizeMetadata(input.Category, input.Severity, input.DisplayMode)
	if err != nil {
		return Announcement{}, err
	}

	item := Announcement{
		ID:                "ANN_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")),
		Title:             title,
		ContentMarkdown:   content,
		Category:          category,
		Severity:          severity,
		DisplayMode:       displayMode,
		Status:            StatusDraft,
		StartsAt:          input.StartsAt,
		EndsAt:            input.EndsAt,
		CreatedBy:         string(actor.UserID),
		CreatedByUserType: int(actor.UserType),
		UpdatedBy:         string(actor.UserID),
		Audiences:         append([]AudienceRule(nil), input.Audiences...),
	}

	switch {
	case actor.Has(auth.CapabilityPlatformAdmin):
		item.PublisherType = PublisherPlatform
		audiences, err := normalizePlatformAudiences(input.Audiences)
		if err != nil {
			return Announcement{}, err
		}
		item.Audiences = audiences
	case actor.Has(auth.CapabilityTenantSelf):
		if strings.TrimSpace(string(actor.TenantID)) == "" {
			return Announcement{}, ErrForbidden
		}
		item.PublisherType = PublisherTenant
		item.PublisherTenantID = string(actor.TenantID)
		item.Audiences = []AudienceRule{{
			Kind:      AudienceEndUser,
			ScopeType: AudienceScopeTenant,
			TenantID:  string(actor.TenantID),
		}}
	default:
		return Announcement{}, ErrForbidden
	}
	return s.repo.CreateDraft(ctx, item)
}

func (s *Service) Publish(ctx context.Context, actor Actor, id string) (Announcement, error) {
	if s.publisher == nil {
		return Announcement{}, ErrUnavailable
	}
	return s.publisher.Publish(ctx, actor, id, time.Now().UTC())
}

func (s *Service) UpdateDraft(ctx context.Context, actor Actor, id string, input DraftInput) (Announcement, error) {
	if s.reader == nil || s.updater == nil {
		return Announcement{}, ErrUnavailable
	}
	item, err := s.reader.GetManaged(ctx, actor, id)
	if err != nil {
		return Announcement{}, err
	}
	if item.Status != StatusDraft {
		return Announcement{}, ErrInvalidTransition
	}
	updated, err := applyDraftInput(actor, item, input)
	if err != nil {
		return Announcement{}, err
	}
	updated.UpdatedBy = string(actor.UserID)
	return s.updater.UpdateDraft(ctx, actor, updated)
}

func applyDraftInput(actor Actor, item Announcement, input DraftInput) (Announcement, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" || len([]rune(title)) > 200 {
		return Announcement{}, ErrInvalidTitle
	}
	content := strings.TrimSpace(input.ContentMarkdown)
	if content == "" || len([]rune(content)) > 50000 {
		return Announcement{}, ErrInvalidContent
	}
	if input.StartsAt != nil && input.EndsAt != nil && !input.StartsAt.Before(*input.EndsAt) {
		return Announcement{}, ErrInvalidSchedule
	}
	category, severity, displayMode, err := normalizeMetadata(input.Category, input.Severity, input.DisplayMode)
	if err != nil {
		return Announcement{}, err
	}
	audiences := append([]AudienceRule(nil), input.Audiences...)
	if actor.Has(auth.CapabilityTenantSelf) {
		if actor.TenantID == "" || item.PublisherType != PublisherTenant || item.PublisherTenantID != string(actor.TenantID) {
			return Announcement{}, ErrForbidden
		}
		audiences = []AudienceRule{{Kind: AudienceEndUser, ScopeType: AudienceScopeTenant, TenantID: string(actor.TenantID)}}
	} else if actor.Has(auth.CapabilityPlatformAdmin) {
		audiences, err = normalizePlatformAudiences(audiences)
		if err != nil {
			return Announcement{}, err
		}
	} else {
		return Announcement{}, ErrForbidden
	}
	item.Title = title
	item.ContentMarkdown = content
	item.Category = category
	item.Severity = severity
	item.DisplayMode = displayMode
	item.StartsAt = input.StartsAt
	item.EndsAt = input.EndsAt
	item.Audiences = audiences
	return item, nil
}

func normalizeMetadata(category Category, severity Severity, displayMode DisplayMode) (Category, Severity, DisplayMode, error) {
	if category == "" {
		category = CategoryGeneral
	}
	if severity == "" {
		severity = SeverityInfo
	}
	if displayMode == "" {
		displayMode = DisplayInbox
	}
	switch category {
	case CategoryGeneral, CategoryMaintenance, CategoryUpgrade, CategoryPricing, CategorySecurity:
	default:
		return "", "", "", ErrInvalidMetadata
	}
	switch severity {
	case SeverityInfo, SeverityImportant, SeverityCritical:
	default:
		return "", "", "", ErrInvalidMetadata
	}
	switch displayMode {
	case DisplayInbox, DisplayPopup:
	default:
		return "", "", "", ErrInvalidMetadata
	}
	return category, severity, displayMode, nil
}

func normalizePlatformAudiences(rules []AudienceRule) ([]AudienceRule, error) {
	if len(rules) == 0 || len(rules) > 500 {
		return nil, ErrInvalidAudience
	}
	seen := make(map[AudienceRule]struct{}, len(rules))
	hasAll := make(map[AudienceKind]bool)
	hasTenant := make(map[AudienceKind]bool)
	out := make([]AudienceRule, 0, len(rules))
	for _, rule := range rules {
		rule.TenantID = strings.TrimSpace(rule.TenantID)
		switch rule.Kind {
		case AudienceAdmin, AudienceTenantUser, AudienceEndUser:
		default:
			return nil, ErrInvalidAudience
		}
		switch rule.ScopeType {
		case AudienceScopeAll:
			if rule.TenantID != "" {
				return nil, ErrInvalidAudience
			}
			hasAll[rule.Kind] = true
		case AudienceScopeTenant:
			if rule.TenantID == "" || rule.Kind == AudienceAdmin {
				return nil, ErrInvalidAudience
			}
			hasTenant[rule.Kind] = true
		default:
			return nil, ErrInvalidAudience
		}
		if _, ok := seen[rule]; ok {
			return nil, ErrInvalidAudience
		}
		seen[rule] = struct{}{}
		out = append(out, rule)
	}
	for kind := range hasAll {
		if hasTenant[kind] {
			return nil, ErrInvalidAudience
		}
	}
	return out, nil
}
