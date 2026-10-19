package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type NATSConfig struct {
	URL        string
	Name       string
	Stream     string
	Subjects   []string
	AckWait    time.Duration
	MaxDeliver int
}

type NATSPublisher struct {
	conn *nats.Conn
	js   nats.JetStreamContext
}

func NewNATSPublisher(cfg NATSConfig) (*NATSPublisher, error) {
	conn, js, err := connect(cfg)
	if err != nil {
		return nil, err
	}
	return &NATSPublisher{conn: conn, js: js}, nil
}

func (p *NATSPublisher) Publish(ctx context.Context, subject string, envelope Envelope) error {
	if p == nil || p.js == nil {
		return ErrPublisherClosed
	}
	if subject == "" || envelope.EventID == "" {
		return errors.New("events: subject and event_id are required")
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("events: marshal envelope: %w", err)
	}
	msg := nats.NewMsg(subject)
	msg.Data = body
	msg.Header.Set(nats.MsgIdHdr, envelope.EventID)
	if _, err = p.js.PublishMsg(msg, nats.Context(ctx)); err != nil {
		return fmt.Errorf("events: publish: %w", err)
	}
	return nil
}

func (p *NATSPublisher) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	err := p.conn.Drain()
	p.conn = nil
	p.js = nil
	return err
}

type Handler func(context.Context, Envelope) error

type NATSSubscriber struct {
	conn *nats.Conn
	js   nats.JetStreamContext
	cfg  NATSConfig
	seen sync.Map
}

func NewNATSSubscriber(cfg NATSConfig) (*NATSSubscriber, error) {
	conn, js, err := connect(cfg)
	if err != nil {
		return nil, err
	}
	return &NATSSubscriber{conn: conn, js: js, cfg: cfg}, nil
}

// Subscribe uses a durable pull consumer. A message is acked only after the
// handler succeeds; malformed messages are terminated instead of retried.
func (s *NATSSubscriber) Subscribe(ctx context.Context, subject, durable string, handler Handler) error {
	if s == nil || s.js == nil || handler == nil {
		return errors.New("events: subscriber and handler are required")
	}
	ackWait := s.cfg.AckWait
	if ackWait <= 0 {
		ackWait = 30 * time.Second
	}
	maxDeliver := s.cfg.MaxDeliver
	if maxDeliver <= 0 {
		maxDeliver = 5
	}
	sub, err := s.js.PullSubscribe(subject, durable, nats.BindStream(streamName(s.cfg)), nats.AckExplicit(), nats.AckWait(ackWait), nats.MaxDeliver(maxDeliver))
	if err != nil {
		return fmt.Errorf("events: subscribe: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	for {
		msgs, fetchErr := sub.Fetch(1, nats.Context(ctx))
		if fetchErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// The nats Go client returns a wrapped context.DeadlineExceeded
			// (not nats.ErrTimeout) when its default fetch timeout fires on a
			// context without an explicit deadline. Treat it as a normal idle
			// tick and keep the consumer loop alive.
			if errors.Is(fetchErr, nats.ErrTimeout) || errors.Is(fetchErr, context.DeadlineExceeded) {
				continue
			}
			return fmt.Errorf("events: fetch: %w", fetchErr)
		}
		for _, msg := range msgs {
			var envelope Envelope
			if err = json.Unmarshal(msg.Data, &envelope); err != nil || envelope.EventID == "" {
				_ = msg.Term()
				continue
			}
			if _, duplicate := s.seen.LoadOrStore(envelope.EventID, struct{}{}); duplicate {
				_ = msg.Ack()
				continue
			}
			if err = handler(ctx, envelope); err != nil {
				s.seen.Delete(envelope.EventID)
				_ = msg.Nak()
				continue
			}
			_ = msg.Ack()
		}
	}
}

func (s *NATSSubscriber) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	err := s.conn.Drain()
	s.conn = nil
	s.js = nil
	return err
}

func connect(cfg NATSConfig) (*nats.Conn, nats.JetStreamContext, error) {
	if cfg.URL == "" {
		return nil, nil, errors.New("events: NATS URL is required")
	}
	name := cfg.Name
	if name == "" {
		name = "inflora-shared"
	}
	conn, err := nats.Connect(cfg.URL, nats.Name(name))
	if err != nil {
		return nil, nil, fmt.Errorf("events: connect NATS: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("events: JetStream: %w", err)
	}
	subjects := cfg.Subjects
	if len(subjects) == 0 {
		subjects = DefaultStreamSubjects
	}
	// Normalize a bare ">" catch-all to the platform's concrete namespace
	// prefixes. A stream that captures ">" (or resolves to it) must be created
	// with NoAck=true, and a NoAck stream never sends publish acknowledgements
	// — which breaks the synchronous PublishMsg below. The "*.>" namespace
	// prefixes in DefaultStreamSubjects are fully ackable, so ">" is never a
	// valid subject here and is rewritten to the platform default set.
	for _, s := range subjects {
		if s == ">" {
			subjects = DefaultStreamSubjects
			break
		}
	}
	info, err := js.StreamInfo(streamName(cfg))
	if err != nil {
		if !errors.Is(err, nats.ErrStreamNotFound) {
			conn.Close()
			return nil, nil, fmt.Errorf("events: inspect stream: %w", err)
		}
		if _, err = js.AddStream(&nats.StreamConfig{Name: streamName(cfg), Subjects: subjects, Storage: nats.FileStorage}); err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("events: ensure stream: %w", err)
		}
		return conn, js, nil
	}
	// Subject drift: a new event family added to DefaultStreamSubjects after
	// the stream's first creation never reaches the deployed stream (AddStream
	// is create-only), and publishes to uncovered subjects fail with "no
	// response from stream". Sync the stream to the current set at boot.
	if !sameSubjects(info.Config.Subjects, subjects) {
		if _, err = js.UpdateStream(&nats.StreamConfig{Name: streamName(cfg), Subjects: subjects}); err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("events: sync stream subjects: %w", err)
		}
	}
	return conn, js, nil
}

// sameSubjects reports whether two subject lists cover the same subjects,
// ignoring order and duplicates.
func sameSubjects(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			return false
		}
	}
	return true
}

func streamName(cfg NATSConfig) string {
	if cfg.Stream == "" {
		return "INFLORA"
	}
	return cfg.Stream
}
