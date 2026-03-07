package store

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Memory store implementations for testing

// MemoryUserStore provides in-memory user storage for testing
type MemoryUserStore struct {
	mu    sync.RWMutex
	users map[uuid.UUID]*User
	links map[string]*UserOAuthLink // keyed by provider:provider_uid
}

func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{
		users: make(map[uuid.UUID]*User),
		links: make(map[string]*UserOAuthLink),
	}
}

func (s *MemoryUserStore) Create(ctx context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	user.ID = uuid.New()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	s.users[user.ID] = user
	return nil
}

func (s *MemoryUserStore) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.users[id], nil
}

func (s *MemoryUserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, nil
}

func (s *MemoryUserStore) Update(ctx context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user.UpdatedAt = time.Now()
	s.users[user.ID] = user
	return nil
}

func (s *MemoryUserStore) Delete(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, id)
	return nil
}

func (s *MemoryUserStore) CreateOAuthLink(ctx context.Context, link *UserOAuthLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := link.Provider + ":" + link.ProviderUID
	s.links[key] = link
	return nil
}

func (s *MemoryUserStore) GetOAuthLink(ctx context.Context, provider, providerUID string) (*UserOAuthLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := provider + ":" + providerUID
	return s.links[key], nil
}

// MemoryOrgStore provides in-memory organization storage for testing
type MemoryOrgStore struct {
	mu      sync.RWMutex
	orgs    map[uuid.UUID]*Organization
	members map[uuid.UUID]map[uuid.UUID]*OrgMember // orgID -> userID -> member
}

func NewMemoryOrgStore() *MemoryOrgStore {
	return &MemoryOrgStore{
		orgs:    make(map[uuid.UUID]*Organization),
		members: make(map[uuid.UUID]map[uuid.UUID]*OrgMember),
	}
}

func (s *MemoryOrgStore) Create(ctx context.Context, org *Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	org.ID = uuid.New()
	org.CreatedAt = time.Now()
	org.UpdatedAt = time.Now()
	s.orgs[org.ID] = org
	s.members[org.ID] = make(map[uuid.UUID]*OrgMember)
	return nil
}

func (s *MemoryOrgStore) GetByID(ctx context.Context, id uuid.UUID) (*Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orgs[id], nil
}

func (s *MemoryOrgStore) GetBySlug(ctx context.Context, slug string) (*Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, org := range s.orgs {
		if org.Slug == slug {
			return org, nil
		}
	}
	return nil, nil
}

func (s *MemoryOrgStore) ListForUser(ctx context.Context, userID uuid.UUID) ([]*Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var orgs []*Organization
	for orgID, memberMap := range s.members {
		if _, exists := memberMap[userID]; exists {
			if org := s.orgs[orgID]; org != nil {
				orgs = append(orgs, org)
			}
		}
	}
	return orgs, nil
}

func (s *MemoryOrgStore) AddMember(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.members[orgID] == nil {
		s.members[orgID] = make(map[uuid.UUID]*OrgMember)
	}
	
	s.members[orgID][userID] = &OrgMember{
		ID:       uuid.New(),
		OrgID:    orgID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
	}
	return nil
}

func (s *MemoryOrgStore) GetMembership(ctx context.Context, orgID, userID uuid.UUID) (*OrgMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if memberMap := s.members[orgID]; memberMap != nil {
		return memberMap[userID], nil
	}
	return nil, nil
}

func (s *MemoryOrgStore) Update(ctx context.Context, org *Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	org.UpdatedAt = time.Now()
	s.orgs[org.ID] = org
	return nil
}

func (s *MemoryOrgStore) Delete(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.orgs, id)
	delete(s.members, id)
	return nil
}

func (s *MemoryOrgStore) ListMembers(ctx context.Context, orgID uuid.UUID, opts ListOpts) ([]*OrgMember, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var members []*OrgMember
	if memberMap := s.members[orgID]; memberMap != nil {
		for _, member := range memberMap {
			members = append(members, member)
		}
	}
	return members, len(members), nil
}

func (s *MemoryOrgStore) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if memberMap := s.members[orgID]; memberMap != nil {
		delete(memberMap, userID)
	}
	return nil
}

func (s *MemoryOrgStore) UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if memberMap := s.members[orgID]; memberMap != nil {
		if member := memberMap[userID]; member != nil {
			member.Role = role
		}
	}
	return nil
}

// MemoryAgentStore provides in-memory agent storage for testing
type MemoryAgentStore struct {
	mu     sync.RWMutex
	agents map[uuid.UUID]*Agent
}

func NewMemoryAgentStore() *MemoryAgentStore {
	return &MemoryAgentStore{
		agents: make(map[uuid.UUID]*Agent),
	}
}

func (s *MemoryAgentStore) Register(ctx context.Context, agent *Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	agent.ID = uuid.New()
	agent.CreatedAt = time.Now()
	agent.UpdatedAt = time.Now()
	s.agents[agent.ID] = agent
	return nil
}

func (s *MemoryAgentStore) GetByID(ctx context.Context, id uuid.UUID) (*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agents[id], nil
}

func (s *MemoryAgentStore) ListByOrg(ctx context.Context, orgID uuid.UUID, opts ListOpts) ([]*Agent, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var agents []*Agent
	for _, agent := range s.agents {
		if agent.OrgID == orgID {
			agents = append(agents, agent)
		}
	}
	return agents, len(agents), nil
}

func (s *MemoryAgentStore) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if agent := s.agents[id]; agent != nil {
		agent.Status = status
		now := time.Now()
		agent.LastHeartbeat = &now
		agent.UpdatedAt = time.Now()
	}
	return nil
}

func (s *MemoryAgentStore) Delete(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, id)
	return nil
}

// Placeholder implementations for other stores
type MemoryEventStore struct{}
func NewMemoryEventStore() *MemoryEventStore { return &MemoryEventStore{} }

type MemoryAPIKeyStore struct{}
func NewMemoryAPIKeyStore() *MemoryAPIKeyStore { return &MemoryAPIKeyStore{} }

type MemoryInvitationStore struct{}
func NewMemoryInvitationStore() *MemoryInvitationStore { return &MemoryInvitationStore{} }

type MemoryAuditStore struct{}
func NewMemoryAuditStore() *MemoryAuditStore { return &MemoryAuditStore{} }