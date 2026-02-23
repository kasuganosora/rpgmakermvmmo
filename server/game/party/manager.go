package party

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"go.uber.org/zap"
)

const maxPartySize = 4
const inviteTimeout = 30 * time.Second

var partyIDCounter int64

func nextPartyID() int64 {
	return atomic.AddInt64(&partyIDCounter, 1)
}

// Party represents an active party of players.
type Party struct {
	ID       int64
	LeaderID int64
	Members  []*player.PlayerSession
	mu       sync.RWMutex
}

// AddMember adds a session to the party. Returns error if full.
func (p *Party) AddMember(s *player.PlayerSession) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.Members) >= maxPartySize {
		return errors.New("party is full")
	}
	p.Members = append(p.Members, s)
	return nil
}

// RemoveMember removes the session for charID from the party.
func (p *Party) RemoveMember(charID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, m := range p.Members {
		if m.CharID == charID {
			p.Members = append(p.Members[:i], p.Members[i+1:]...)
			return
		}
	}
}

// Size returns the current number of members.
func (p *Party) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.Members)
}

// GetNearbyMembers returns party members on the same map within radius tiles of (x, y).
func (p *Party) GetNearbyMembers(mapID, x, y, radius int) []*player.PlayerSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var result []*player.PlayerSession
	for _, m := range p.Members {
		if m.MapID != mapID {
			continue
		}
		mx, my, _ := m.Position()
		dx := mx - x
		dy := my - y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dx+dy <= radius {
			result = append(result, m)
		}
	}
	return result
}

// BroadcastUpdate sends party_update to all members.
func (p *Party) BroadcastUpdate() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	membersData := make([]map[string]interface{}, 0, len(p.Members))
	for _, m := range p.Members {
		mx, my, _ := m.Position()
		hp, maxHP, mp, maxMP := m.Stats()
		membersData = append(membersData, map[string]interface{}{
			"char_id": m.CharID,
			"name":    m.CharName,
			"hp":      hp,
			"max_hp":  maxHP,
			"mp":      mp,
			"max_mp":  maxMP,
			"map_id":  m.MapID,
			"x":       mx,
			"y":       my,
			"online":  true,
		})
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"party_id":  p.ID,
		"leader_id": p.LeaderID,
		"members":   membersData,
	})
	pkt, _ := json.Marshal(&player.Packet{Type: "party_update", Payload: payload})
	for _, m := range p.Members {
		m.SendRaw(pkt)
	}
}

// Manager manages all active parties and pending invites.
type Manager struct {
	mu      sync.RWMutex
	parties map[int64]*Party              // partyID → Party
	byChar  map[int64]int64               // charID → partyID
	invites map[int64]*pendingInvite      // targetCharID → invite
	logger  *zap.Logger
}

type pendingInvite struct {
	PartyID  int64
	InviterID int64
	ExpiresAt time.Time
}

// NewManager creates a new party Manager.
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		parties: make(map[int64]*Party),
		byChar:  make(map[int64]int64),
		invites: make(map[int64]*pendingInvite),
		logger:  logger,
	}
}

// InvitePlayer sends a party invite from inviter to target.
func (m *Manager) InvitePlayer(inviter, target *player.PlayerSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.byChar[target.CharID]; ok {
		return errors.New("target already in a party")
	}
	m.invites[target.CharID] = &pendingInvite{
		InviterID: inviter.CharID,
		ExpiresAt: time.Now().Add(inviteTimeout),
	}
	// Send invite request to target.
	payload, _ := json.Marshal(map[string]interface{}{
		"from_id":   inviter.CharID,
		"from_name": inviter.CharName,
	})
	target.Send(&player.Packet{Type: "party_invite_request", Payload: payload})
	return nil
}

// AcceptInvite processes a party invite acceptance from s.
// inviter is the session of the player who sent the invite (needed to add them to the party).
func (m *Manager) AcceptInvite(s *player.PlayerSession, inviter *player.PlayerSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inv, ok := m.invites[s.CharID]
	if !ok || time.Now().After(inv.ExpiresAt) {
		delete(m.invites, s.CharID)
		return errors.New("no valid invite")
	}
	delete(m.invites, s.CharID)

	// Find or create party.
	if inv.PartyID == 0 {
		// Create a new party with the inviter as leader and first member.
		inv.PartyID = nextPartyID()
		p := &Party{ID: inv.PartyID, LeaderID: inv.InviterID, Members: []*player.PlayerSession{inviter}}
		m.parties[inv.PartyID] = p
		m.byChar[inv.InviterID] = inv.PartyID
		m.logger.Info("party created", zap.Int64("party_id", inv.PartyID))
	}

	p := m.parties[inv.PartyID]
	if err := p.AddMember(s); err != nil {
		return err
	}
	m.byChar[s.CharID] = inv.PartyID
	go p.BroadcastUpdate()
	return nil
}

// DeclineInvite removes a pending invite for the given character.
func (m *Manager) DeclineInvite(charID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.invites, charID)
}

// LeaveParty removes s from their current party.
// The manager lock is held throughout the removal so that AcceptInvite cannot
// race between the size-check and the party deletion (TOCTOU fix).
// Lock order is always m.mu → p.mu, matching AcceptInvite, so no deadlock.
func (m *Manager) LeaveParty(s *player.PlayerSession) {
	m.mu.Lock()
	partyID, ok := m.byChar[s.CharID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.byChar, s.CharID)
	p := m.parties[partyID]
	if p == nil {
		m.mu.Unlock()
		return
	}
	p.RemoveMember(s.CharID)
	if p.Size() == 0 {
		delete(m.parties, partyID)
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	go p.BroadcastUpdate()
}

// CleanupInvites removes any pending invites sent to or from the given charID.
func (m *Manager) CleanupInvites(charID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Remove invite targeting this character.
	delete(m.invites, charID)
	// Remove invites sent by this character.
	for targetID, inv := range m.invites {
		if inv.InviterID == charID {
			delete(m.invites, targetID)
		}
	}
}

// GetParty returns the Party a character belongs to, or nil.
func (m *Manager) GetParty(charID int64) *Party {
	m.mu.RLock()
	defer m.mu.RUnlock()
	partyID, ok := m.byChar[charID]
	if !ok {
		return nil
	}
	return m.parties[partyID]
}
