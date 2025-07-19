package raft

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"
)

// NodeState represents the state of a Raft node
type NodeState int

const (
	Follower NodeState = iota
	Candidate
	Leader
)

func (s NodeState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// LogEntry represents a log entry in Raft
type LogEntry struct {
	Term    int64       `json:"term"`
	Index   int64       `json:"index"`
	Command interface{} `json:"command"`
}

// VoteRequest represents a vote request in Raft
type VoteRequest struct {
	Term         int64  `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex int64  `json:"last_log_index"`
	LastLogTerm  int64  `json:"last_log_term"`
}

// VoteResponse represents a vote response
type VoteResponse struct {
	Term        int64 `json:"term"`
	VoteGranted bool  `json:"vote_granted"`
}

// AppendEntriesRequest represents an append entries request
type AppendEntriesRequest struct {
	Term         int64       `json:"term"`
	LeaderID     string      `json:"leader_id"`
	PrevLogIndex int64       `json:"prev_log_index"`
	PrevLogTerm  int64       `json:"prev_log_term"`
	Entries      []*LogEntry `json:"entries"`
	LeaderCommit int64       `json:"leader_commit"`
}

// AppendEntriesResponse represents an append entries response
type AppendEntriesResponse struct {
	Term    int64 `json:"term"`
	Success bool  `json:"success"`
}

// RaftNode represents a Raft consensus node
type RaftNode struct {
	mu          sync.RWMutex
	nodeID      string
	peers       []string
	state       NodeState
	currentTerm int64
	votedFor    string
	log         []*LogEntry
	commitIndex int64
	lastApplied int64

	// Leader state
	nextIndex  map[string]int64
	matchIndex map[string]int64

	// Channels and timers
	electionTimer  *time.Timer
	heartbeatTimer *time.Timer
	voteCh         chan bool
	appendCh       chan bool
	
	// RPC interface
	rpc RaftRPC
	
	// Application state machine
	applyCh chan *LogEntry
	
	// Shutdown
	shutdown chan struct{}
	running  bool
}

// RaftRPC defines the interface for Raft RPC operations
type RaftRPC interface {
	RequestVote(ctx context.Context, peer string, req *VoteRequest) (*VoteResponse, error)
	AppendEntries(ctx context.Context, peer string, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
}

// NewRaftNode creates a new Raft node
func NewRaftNode(nodeID string, peers []string, rpc RaftRPC) *RaftNode {
	node := &RaftNode{
		nodeID:      nodeID,
		peers:       peers,
		state:       Follower,
		currentTerm: 0,
		log:         make([]*LogEntry, 0),
		nextIndex:   make(map[string]int64),
		matchIndex:  make(map[string]int64),
		voteCh:      make(chan bool, 1),
		appendCh:    make(chan bool, 1),
		rpc:         rpc,
		applyCh:     make(chan *LogEntry, 100),
		shutdown:    make(chan struct{}),
	}
	
	return node
}

// Start starts the Raft node
func (rn *RaftNode) Start() {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	if rn.running {
		return
	}
	
	rn.running = true
	rn.resetElectionTimer()
	
	go rn.run()
}

// Stop stops the Raft node
func (rn *RaftNode) Stop() {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	if !rn.running {
		return
	}
	
	rn.running = false
	close(rn.shutdown)
	
	if rn.electionTimer != nil {
		rn.electionTimer.Stop()
	}
	if rn.heartbeatTimer != nil {
		rn.heartbeatTimer.Stop()
	}
}

// run is the main event loop for the Raft node
func (rn *RaftNode) run() {
	for {
		select {
		case <-rn.shutdown:
			return
		default:
			rn.mu.RLock()
			state := rn.state
			rn.mu.RUnlock()
			
			switch state {
			case Follower:
				rn.runFollower()
			case Candidate:
				rn.runCandidate()
			case Leader:
				rn.runLeader()
			}
		}
	}
}

// runFollower handles follower state
func (rn *RaftNode) runFollower() {
	select {
	case <-rn.electionTimer.C:
		rn.mu.Lock()
		rn.state = Candidate
		rn.mu.Unlock()
		
	case <-rn.appendCh:
		rn.resetElectionTimer()
		
	case <-rn.shutdown:
		return
	}
}

// runCandidate handles candidate state
func (rn *RaftNode) runCandidate() {
	rn.mu.Lock()
	rn.currentTerm++
	rn.votedFor = rn.nodeID
	rn.resetElectionTimer()
	rn.mu.Unlock()
	
	votes := 1 // Vote for self
	majority := len(rn.peers)/2 + 1
	
	// Request votes from all peers
	for _, peer := range rn.peers {
		if peer == rn.nodeID {
			continue
		}
		
		go func(peer string) {
			rn.mu.RLock()
			req := &VoteRequest{
				Term:        rn.currentTerm,
				CandidateID: rn.nodeID,
			}
			if len(rn.log) > 0 {
				req.LastLogIndex = rn.log[len(rn.log)-1].Index
				req.LastLogTerm = rn.log[len(rn.log)-1].Term
			}
			rn.mu.RUnlock()
			
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			
			resp, err := rn.rpc.RequestVote(ctx, peer, req)
			if err != nil {
				return
			}
			
			rn.mu.Lock()
			if resp.Term > rn.currentTerm {
				rn.currentTerm = resp.Term
				rn.votedFor = ""
				rn.state = Follower
				rn.mu.Unlock()
				return
			}
			rn.mu.Unlock()
			
			if resp.VoteGranted {
				select {
				case rn.voteCh <- true:
				default:
				}
			}
		}(peer)
	}
	
	// Wait for votes or timeout
	voteTimer := time.NewTimer(150 * time.Millisecond)
	defer voteTimer.Stop()
	
	for votes < majority {
		select {
		case <-rn.voteCh:
			votes++
			
		case <-voteTimer.C:
			return // Election timeout, stay candidate
			
		case <-rn.appendCh:
			rn.mu.Lock()
			rn.state = Follower
			rn.mu.Unlock()
			return
			
		case <-rn.shutdown:
			return
		}
	}
	
	// Won election, become leader
	rn.mu.Lock()
	rn.state = Leader
	rn.initializeLeaderState()
	rn.mu.Unlock()
	
	log.Printf("Node %s became leader for term %d", rn.nodeID, rn.currentTerm)
}

// runLeader handles leader state
func (rn *RaftNode) runLeader() {
	rn.sendHeartbeats()
	
	rn.heartbeatTimer = time.NewTimer(50 * time.Millisecond)
	defer rn.heartbeatTimer.Stop()
	
	select {
	case <-rn.heartbeatTimer.C:
		// Continue as leader
		
	case <-rn.appendCh:
		// Received append entries, step down
		rn.mu.Lock()
		rn.state = Follower
		rn.mu.Unlock()
		
	case <-rn.shutdown:
		return
	}
}

// initializeLeaderState initializes leader-specific state
func (rn *RaftNode) initializeLeaderState() {
	lastLogIndex := int64(0)
	if len(rn.log) > 0 {
		lastLogIndex = rn.log[len(rn.log)-1].Index
	}
	
	for _, peer := range rn.peers {
		if peer != rn.nodeID {
			rn.nextIndex[peer] = lastLogIndex + 1
			rn.matchIndex[peer] = 0
		}
	}
}

// sendHeartbeats sends heartbeat messages to all followers
func (rn *RaftNode) sendHeartbeats() {
	rn.mu.RLock()
	term := rn.currentTerm
	rn.mu.RUnlock()
	
	for _, peer := range rn.peers {
		if peer == rn.nodeID {
			continue
		}
		
		go func(peer string) {
			req := &AppendEntriesRequest{
				Term:     term,
				LeaderID: rn.nodeID,
			}
			
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			
			_, err := rn.rpc.AppendEntries(ctx, peer, req)
			if err != nil {
				log.Printf("Failed to send heartbeat to %s: %v", peer, err)
			}
		}(peer)
	}
}

// resetElectionTimer resets the election timeout with random jitter
func (rn *RaftNode) resetElectionTimer() {
	if rn.electionTimer != nil {
		rn.electionTimer.Stop()
	}
	
	// Random timeout between 150-300ms
	timeout := time.Duration(150+rand.Intn(150)) * time.Millisecond
	rn.electionTimer = time.NewTimer(timeout)
}

// IsLeader returns true if this node is the current leader
func (rn *RaftNode) IsLeader() bool {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	return rn.state == Leader
}

// GetState returns the current state of the node
func (rn *RaftNode) GetState() (NodeState, int64) {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	return rn.state, rn.currentTerm
}

// RequestVote handles vote requests from other nodes
func (rn *RaftNode) RequestVote(req *VoteRequest) *VoteResponse {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	// Reply false if term < currentTerm
	if req.Term < rn.currentTerm {
		return &VoteResponse{
			Term:        rn.currentTerm,
			VoteGranted: false,
		}
	}
	
	// If term > currentTerm, convert to follower
	if req.Term > rn.currentTerm {
		rn.currentTerm = req.Term
		rn.votedFor = ""
		rn.state = Follower
	}
	
	// Check if we can vote for this candidate
	canVote := (rn.votedFor == "" || rn.votedFor == req.CandidateID)
	
	// Check if candidate's log is at least as up-to-date as ours
	logUpToDate := true
	if len(rn.log) > 0 {
		lastLog := rn.log[len(rn.log)-1]
		if req.LastLogTerm < lastLog.Term ||
			(req.LastLogTerm == lastLog.Term && req.LastLogIndex < lastLog.Index) {
			logUpToDate = false
		}
	}
	
	voteGranted := canVote && logUpToDate
	if voteGranted {
		rn.votedFor = req.CandidateID
		rn.resetElectionTimer()
	}
	
	return &VoteResponse{
		Term:        rn.currentTerm,
		VoteGranted: voteGranted,
	}
}

// AppendEntries handles append entries requests from the leader
func (rn *RaftNode) AppendEntries(req *AppendEntriesRequest) *AppendEntriesResponse {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	
	// Reply false if term < currentTerm
	if req.Term < rn.currentTerm {
		return &AppendEntriesResponse{
			Term:    rn.currentTerm,
			Success: false,
		}
	}
	
	// Convert to follower if term is higher
	if req.Term >= rn.currentTerm {
		rn.currentTerm = req.Term
		rn.state = Follower
		rn.votedFor = ""
		
		// Signal that we received a valid append entries
		select {
		case rn.appendCh <- true:
		default:
		}
	}
	
	return &AppendEntriesResponse{
		Term:    rn.currentTerm,
		Success: true,
	}
}
