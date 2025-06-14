package terminal

import (
	"fmt"
	"io"
	"sync"
	"time"
)

type Spinner struct {
	frames   []string
	interval time.Duration
	message  string
	writer   io.Writer
	active   bool
	mu       sync.Mutex
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func NewSpinner(writer io.Writer, message string) *Spinner {
	return &Spinner{
		frames:   []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
		interval: 500 * time.Millisecond,
		message:  message,
		writer:   writer,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func NewSpinnerWithFrames(writer io.Writer, message string, frames []string, interval time.Duration) *Spinner {
	return &Spinner{
		frames:   frames,
		interval: interval,
		message:  message,
		writer:   writer,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.mu.Unlock()

	go s.spin()
}

func (s *Spinner) Stop(completionMessage string) {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.mu.Unlock()

	close(s.stopCh)
	<-s.doneCh

	fmt.Fprintf(s.writer, "\r\033[K")
	if completionMessage != "" {
		fmt.Fprintf(s.writer, "%s\n", completionMessage)
	}
}

func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	s.message = message
	s.mu.Unlock()
}

func (s *Spinner) spin() {
	defer close(s.doneCh)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	frameIndex := 0

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			frame := s.frames[frameIndex]
			message := s.message
			s.mu.Unlock()

			fmt.Fprintf(s.writer, "\r%s %s", frame, message)
			frameIndex = (frameIndex + 1) % len(s.frames)
		}
	}
}

func SpinnerFunc(writer io.Writer, message string, fn func() error) error {
	spinner := NewSpinner(writer, message)
	spinner.Start()

	err := fn()

	if err != nil {
		spinner.Stop(fmt.Sprintf("✗ %s", message))
	} else {
		spinner.Stop(fmt.Sprintf("✓ %s", message))
	}

	return err
}

func SpinnerFuncWithCustomCompletion(writer io.Writer, message string, successMsg, errorMsg string, fn func() error) error {
	spinner := NewSpinner(writer, message)
	spinner.Start()

	err := fn()

	if err != nil {
		spinner.Stop(errorMsg)
	} else {
		spinner.Stop(successMsg)
	}

	return err
}
