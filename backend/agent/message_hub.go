package agent

import (
	"iter"
	"sync"

	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	"github.com/google/uuid"
	"github.com/maypok86/otter"
)

type MessageStream iter.Seq2[model.ContentBlock, error]

// MessageBlockType indicates whether a block is a delta or a complete message
type MessageBlockType string

const (
	// MessageBlockTypeDelta represents a partial message block
	MessageBlockTypeDelta MessageBlockType = "delta"
	// MessageBlockTypeComplete represents a complete message block
	MessageBlockTypeComplete MessageBlockType = "complete"
)

// MessageBlock wraps a content block with metadata
type MessageBlock struct {
	Block    *types.MessageContentBlock
	Type     MessageBlockType
	Received map[string]bool // tracks which subscribers have received this block
}

// MessageHub manages message blocks and their delivery to subscribers
type MessageHub struct {
	// Cache to store message blocks by taskID
	messages *otter.Cache[uuid.UUID, []*MessageBlock]
	
	// Track whether a complete message has been sent for each task
	completeSent *otter.Cache[uuid.UUID, bool]
	
	// Track subscribers by taskID
	subscribers map[uuid.UUID]map[string]struct{}
	
	// Mutex for thread safety
	mu sync.RWMutex
}

// NewMessageHub creates a new MessageHub instance
func NewMessageHub() (*MessageHub, error) {
	messagesCache, err := otter.MustBuilder[uuid.UUID, []*MessageBlock](1000).
		Build()
	if err != nil {
		return nil, err
	}
	
	completeSentCache, err := otter.MustBuilder[uuid.UUID, bool](1000).
		Build()
	if err != nil {
		return nil, err
	}
	
	return &MessageHub{
		messages:     &messagesCache,
		completeSent: &completeSentCache,
		subscribers:  make(map[uuid.UUID]map[string]struct{}),
	}, nil
}

// AppendBlock adds a new message block to the hub
func (h *MessageHub) Publish(taskID uuid.UUID, block *types.MessageContentBlock, isComplete bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	blockType := MessageBlockTypeDelta
	if isComplete {
		blockType = MessageBlockTypeComplete
	}
	
	messageBlock := &MessageBlock{
		Block:    block,
		Type:     blockType,
		Received: make(map[string]bool),
	}
	
	// Get existing blocks or create a new slice
	blocks, found := h.messages.Get(taskID)
	if !found {
		blocks = []*MessageBlock{}
	}
	
	// Append the new block
	blocks = append(blocks, messageBlock)
	h.messages.Set(taskID, blocks)
	
	// If this is a complete message, mark it as such
	if isComplete {
		h.completeSent.Set(taskID, true)
		
		// Check if we can clean up
		h.tryCleanup(taskID)
	}
}

// Subscribe registers a subscriber for a task
func (h *MessageHub) Subscribe(taskID uuid.UUID, subscriberID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	subs, exists := h.subscribers[taskID]
	if !exists {
		subs = make(map[string]struct{})
		h.subscribers[taskID] = subs
	}
	
	subs[subscriberID] = struct{}{}
}

// Unsubscribe removes a subscriber for a task
func (h *MessageHub) Unsubscribe(taskID uuid.UUID, subscriberID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	subs, exists := h.subscribers[taskID]
	if !exists {
		return
	}
	
	delete(subs, subscriberID)
	
	// If no more subscribers, clean up the entry
	if len(subs) == 0 {
		delete(h.subscribers, taskID)
	}
	
	// Try to clean up messages if possible
	h.tryCleanup(taskID)
}

// tryCleanup checks if all subscribers have received the complete message
// and deletes all messages for the task if they have
func (h *MessageHub) tryCleanup(taskID uuid.UUID) {
	// Check if a complete message has been sent
	completeSent, found := h.completeSent.Get(taskID)
	if !found || !completeSent {
		return
	}
	
	// Get the subscribers for this task
	subs, exists := h.subscribers[taskID]
	if !exists || len(subs) == 0 {
		// No subscribers, we can clean up
		h.messages.Delete(taskID)
		h.completeSent.Delete(taskID)
		return
	}
	
	// Get the blocks for this task
	blocks, found := h.messages.Get(taskID)
	if !found {
		return
	}
	
	// Find the complete message block
	var completeBlock *MessageBlock
	for _, block := range blocks {
		if block.Type == MessageBlockTypeComplete {
			completeBlock = block
			break
		}
	}
	
	if completeBlock == nil {
		return
	}
	
	// Check if all subscribers have received the complete message
	allReceived := true
	for subscriberID := range subs {
		if !completeBlock.Received[subscriberID] {
			allReceived = false
			break
		}
	}
	
	// If all subscribers have received the complete message, clean up
	if allReceived {
		h.messages.Delete(taskID)
		h.completeSent.Delete(taskID)
	}
}

// markBlockReceived marks a block as received by a subscriber
func (h *MessageHub) markBlockReceived(taskID uuid.UUID, blockIndex int, subscriberID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	blocks, found := h.messages.Get(taskID)
	if !found || blockIndex >= len(blocks) {
		return
	}
	
	blocks[blockIndex].Received[subscriberID] = true
	
	// Try to clean up if this was the last block
	if blocks[blockIndex].Type == MessageBlockTypeComplete {
		h.tryCleanup(taskID)
	}
}

// Stream returns a stream of content blocks for a task
func (h *MessageHub) Stream(taskID uuid.UUID, subscriberID string) MessageStream {
	return func(yield func(model.ContentBlock, error) bool) {
		h.mu.RLock()
		blocks, found := h.messages.Get(taskID)
		if !found {
			h.mu.RUnlock()
			return
		}
		
		// Register as a subscriber if not already
		h.Subscribe(taskID, subscriberID)
		h.mu.RUnlock()
		
		// Stream the blocks
		for i, block := range blocks {
			// Convert to model.ContentBlock
			contentBlock := &model.TextContentBlock{
				Text: block.Block.Text,
			}
			
			if !yield(contentBlock, nil) {
				break
			}
			
			// Mark as received
			h.markBlockReceived(taskID, i, subscriberID)
		}
	}
}
