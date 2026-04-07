package main

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// BufferedStreamer accumulates text chunks and streams them
// Use this when you receive multiple text blocks and want to stream them as one response
//
// Thread-safety: Add() can be called concurrently from multiple goroutines.
// Done() is idempotent and can be called multiple times safely.
//
// Trade-off: Uses non-blocking sends to avoid blocking producers. If chunks are
// added faster than the goroutine can drain them (extremely rare with 5000-slot buffer),
// chunks will be dropped. This prevents blocking the caller during Telegram backpressure.
type BufferedStreamer struct {
	config         *Config
	chatID         int64
	threadID       int64
	enabled        bool
	textBuilder    strings.Builder
	chunkChan      chan string
	flushTimer     *time.Timer
	flushInterval  time.Duration
	doneChan       chan bool         // Signals goroutine to stop
	doneChanOnce   sync.Once         // Ensures doneChan is only closed once
	finalizeDone   chan struct{}     // Signals when finalization is complete
	lastFlush      time.Time
	minChunkSize   int
	finalMessageID int64             // Stores the message ID from finalization
	finalizeErr    error             // Stores finalization error
	state          atomic.Int32      // 0=not started, 1=running, 2=finalized
	mu             sync.Mutex        // Protects finalMessageID, finalizeErr, and concurrent Add
}

// NewBufferedStreamer creates a new buffered streamer
func NewBufferedStreamer(config *Config, chatID int64, threadID int64, enabled bool) *BufferedStreamer {
	bs := &BufferedStreamer{
		config:        config,
		chatID:        chatID,
		threadID:      threadID,
		enabled:       enabled,
		chunkChan:     make(chan string, 5000), // Large buffer to prevent chunk loss under load
		                                         // Non-blocking send drops chunks when full to avoid blocking producers
		                                         // Trade-off: possible data loss vs blocking command execution
		                                         // 5000 slots should be sufficient for typical AI response streaming
		doneChan:      make(chan bool),
		finalizeDone:  make(chan struct{}),
		flushInterval: 100 * time.Millisecond, // Flush every 100ms
		minChunkSize:  10,                    // Or after 10 characters
	}
	bs.state.Store(0) // 0 = not started
	return bs
}

// Start begins the background streaming goroutine
func (bs *BufferedStreamer) Start() {
	if !bs.enabled {
		return // Streaming disabled
	}

	// Try to transition from "not started" (0) to "running" (1)
	// If this fails, we're already finalized (2) or running (1)
	if !bs.state.CompareAndSwap(0, 1) {
		return // Already started or finalized
	}

	go func() {
		defer bs.state.Store(2) // Transition to finalized when goroutine exits
		defer close(bs.finalizeDone)

		ticker := time.NewTicker(bs.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case chunk := <-bs.chunkChan:
				bs.textBuilder.WriteString(chunk)
				bs.lastFlush = time.Now()

				// Flush if we have enough content or timer fires
				if bs.textBuilder.Len() >= bs.minChunkSize {
					bs.sendDraft()
				}

			case <-ticker.C:
				// Periodic flush
				if time.Since(bs.lastFlush) >= bs.flushInterval && bs.textBuilder.Len() > 0 {
					bs.sendDraft()
				}

			case <-bs.doneChan:
				// Done - give in-flight Add() calls time to observe the shutdown
				time.Sleep(10 * time.Millisecond)

				// Drain remaining chunks that were already in-flight
				// Then finalize - the defer will set state = 2 on goroutine exit
				for {
					select {
					case chunk := <-bs.chunkChan:
						bs.textBuilder.WriteString(chunk)
					default:
						// No more chunks - finalize and exit
						bs.finalize()
						return
					}
				}
			}
		}
	}()
}

// Add adds a text chunk to the stream
// Safe for concurrent use - multiple goroutines can call Add() simultaneously
func (bs *BufferedStreamer) Add(text string) {
	if !bs.enabled {
		// Streaming disabled - just accumulate in buffer
		bs.mu.Lock()
		bs.textBuilder.WriteString(text)
		bs.mu.Unlock()
		return
	}

	// Check current state
	state := bs.state.Load()

	if state == 1 {
		// Goroutine is running - try non-blocking send
		// If channel is full, drop the chunk to avoid blocking the producer
		// Trade-off: Possible data loss vs blocking command execution
		// The 5000-slot buffer should prevent this in normal operation
		select {
		case bs.chunkChan <- text:
			// Chunk sent successfully
			return
		default:
			// Channel full - drop chunk to avoid blocking producer
			// This is rare with 5000 buffer and should only occur under extreme load
			return
		}
	}

	// State is 0 (not started) or 2 (finalized)
	if state == 2 {
		// Already finalized - drop the chunk
		return
	}

	// State is 0 (not started) - write to buffer with mutex
	// Double-check state after acquiring mutex
	bs.mu.Lock()
	if bs.state.Load() != 0 {
		bs.mu.Unlock()
		return
	}
	bs.textBuilder.WriteString(text)
	bs.mu.Unlock()
}

// sendDraft sends the current accumulated text as a draft update
func (bs *BufferedStreamer) sendDraft() {
	if bs.textBuilder.Len() == 0 {
		return
	}

	currentText := bs.textBuilder.String()
	if err := sendDraftMessage(bs.config, bs.chatID, bs.threadID, currentText); err != nil {
		// Draft failures are non-critical
	}
}

// finalize converts the draft to a permanent message and stores the message ID
func (bs *BufferedStreamer) finalize() {
	finalText := bs.textBuilder.String()
	if finalText == "" {
		return
	}

	msgID, err := finalizeStream(bs.config, bs.chatID, bs.threadID, finalText, "Markdown")

	// Store the message ID and error for Done() to retrieve
	bs.mu.Lock()
	bs.finalMessageID = msgID
	bs.finalizeErr = err
	bs.mu.Unlock()
}

// Done finalizes the stream and returns the message ID
func (bs *BufferedStreamer) Done() (int64, error) {
	if !bs.enabled {
		// Not streaming enabled - send normally
		// Check if already finalized first for idempotency
		if bs.state.Load() == 2 {
			bs.mu.Lock()
			defer bs.mu.Unlock()
			return bs.finalMessageID, bs.finalizeErr
		}

		bs.mu.Lock()
		defer bs.mu.Unlock()

		if bs.textBuilder.Len() == 0 {
			bs.state.Store(2) // Transition to finalized
			return 0, nil
		}
		text := bs.textBuilder.String()

		// Send while holding lock to prevent concurrent Add() modifications
		// This blocks Add() during the network call, preventing race condition
		msgID, err := sendMessageGetID(bs.config, bs.chatID, bs.threadID, text)

		// Store result for idempotency
		bs.finalMessageID = msgID
		bs.finalizeErr = err

		// Mark as finalized to prevent further Add() writes
		bs.state.Store(2)
		return msgID, err
	}

	// Try to transition from "not started" (0) to "finalized" (2)
	// This handles the case where Done() is called before Start()
	if bs.state.CompareAndSwap(0, 2) {
		// Successfully transitioned - finalize directly
		bs.mu.Lock()
		defer bs.mu.Unlock()

		if bs.textBuilder.Len() == 0 {
			return 0, nil
		}

		text := bs.textBuilder.String()
		msgID, err := finalizeStream(bs.config, bs.chatID, bs.threadID, text, "Markdown")
		bs.finalMessageID = msgID
		bs.finalizeErr = err
		return msgID, err
	}

	// Check if already finalized (state == 2)
	// This handles the case where Done() is called multiple times
	if bs.state.Load() == 2 {
		// Already finalized - return stored message ID and error
		bs.mu.Lock()
		defer bs.mu.Unlock()
		return bs.finalMessageID, bs.finalizeErr
	}

	// State is 1 (running) - signal goroutine to finalize
	// Use sync.Once to ensure doneChan is only closed once
	bs.doneChanOnce.Do(func() {
		close(bs.doneChan)
	})

	// Wait for finalization to complete (proper synchronization)
	<-bs.finalizeDone

	// Return the stored message ID and error from finalize()
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.finalMessageID, bs.finalizeErr
}
