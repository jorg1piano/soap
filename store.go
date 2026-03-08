package main

import (
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// Store provides NATS pub/sub connectivity
type Store struct {
	nc *nats.Conn
	ns *server.Server
}

// NewServerStore creates a Store that starts an embedded NATS server
func NewServerStore() (*Store, error) {
	opts := &server.Options{
		Port:   natsPort,
		NoLog:  true,
		NoSigs: true,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("new nats server: %w", err)
	}
	ns.Start()

	if !ns.ReadyForConnections(100 * time.Millisecond) {
		return nil, fmt.Errorf("nats server not ready after 100ms")
	}

	os.WriteFile(portFile, []byte(fmt.Sprintf("%d", natsPort)), 0644)

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	return &Store{nc: nc, ns: ns}, nil
}

// NewClientStore creates a Store that connects to an existing NATS server
func NewClientStore() (*Store, error) {
	url := fmt.Sprintf("nats://127.0.0.1:%d", natsPort)

	var nc *nats.Conn
	var err error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		nc, err = nats.Connect(url, nats.Timeout(2*time.Second))
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("nats connect to %s after %d retries: %w", url, maxRetries, err)
	}

	return &Store{nc: nc}, nil
}

// Close closes the NATS connection and shuts down the embedded server if running
func (s *Store) Close() {
	s.nc.Close()
	if s.ns != nil {
		s.ns.Shutdown()
		os.Remove(portFile)
	}
}

// Publish publishes a message to a NATS subject
func (s *Store) Publish(subject string, data []byte) error {
	return s.nc.Publish(subject, data)
}
