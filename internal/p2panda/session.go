// Package p2panda wraps the p2panda-gobject bindings with a channel-oriented
// interface suitable for use from worker goroutines. All interactions with the
// underlying library are marshalled onto the GLib main loop via glib.IdleAdd
// because the bindings emit signals and callbacks on the main context.
package p2panda

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"codeberg.org/puregotk/puregotk/examples/p2panda-gobject-go/p2panda"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"github.com/rs/zerolog/log"
)

const PubkeySize = 32

// Message is a frame received from the topic, annotated with sender info.
type Message struct {
	From      string // Hex-encoded sender public key
	Data      []byte
	Ephemeral bool
}

// PeerEvent signals when a previously-unseen peer sent their first message
// (Joined=true) or — currently — is just a new observation. The underlying
// p2panda library does not expose reliable disconnect events without using the
// sync-started/sync-ended signals, so Joined=false is unused for now but kept
// for API symmetry.
type PeerEvent struct {
	Pubkey string
	Joined bool
}

// Session is a joined p2panda topic. Open() must be called before Publish/Messages.
type Session struct {
	relayURL   string
	networkHex string
	topicHex   string
	bootstrap  string // Optional bootstrap peer pubkey hex

	privateKey *p2panda.PrivateKey
	ownPubHex  string
	networkID  *p2panda.NetworkId
	topicID    *p2panda.TopicId
	relayURI   *glib.Uri
	bootstrapN *p2panda.NodeId
	node       *p2panda.Node
	topic      *p2panda.Topic

	// Keep the signal callbacks alive for the lifetime of the session so GC
	// doesn't collect them while p2panda still holds the references.
	onMsgCB *func(p2panda.Topic, uintptr, uintptr, uintptr) bool
	onEphCB *func(p2panda.Topic, uintptr, uintptr, uintptr)

	messages chan Message
	peers    chan PeerEvent

	mu       sync.Mutex
	seenPeer map[string]struct{}

	closeOnce sync.Once
	closed    chan struct{}
}

// hexTo32 decodes a 64-char hex string into a 32-byte array.
func hexTo32(s string) ([32]byte, error) {
	var out [32]byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	if len(b) != 32 {
		return out, fmt.Errorf("expected 32 bytes, got %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}

// ptrHex reads n bytes from a raw pointer and hex-encodes them.
func ptrHex(ptr uintptr, n int) string {
	return hex.EncodeToString(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), n))
}

// NewSession prepares a session descriptor. It does not open the underlying
// node or topic — call Open for that.
func NewSession(relayURL, networkHex, topicHex, bootstrapHex string) *Session {
	return &Session{
		relayURL:   relayURL,
		networkHex: networkHex,
		topicHex:   topicHex,
		bootstrap:  bootstrapHex,
		messages:   make(chan Message, 64),
		peers:     make(chan PeerEvent, 16),
		seenPeer:   map[string]struct{}{},
		closed:     make(chan struct{}),
	}
}

// NodeIDHex returns the hex-encoded public key of this session's node.
// Only valid after Open() has returned successfully.
func (s *Session) NodeIDHex() string {
	return s.ownPubHex
}

// Messages returns a channel that receives messages from remote peers. Messages
// from this node are filtered out.
func (s *Session) Messages() <-chan Message { return s.messages }

// Peers returns a channel that receives a PeerEvent the first time we see a
// message from a previously-unknown peer.
func (s *Session) Peers() <-chan PeerEvent { return s.peers }

// Done returns a channel that is closed when the session is closed.
func (s *Session) Done() <-chan struct{} { return s.closed }

// Open spawns the node and topic. It blocks until both are ready or ctx is
// cancelled. Must be called from outside the GLib main thread — it uses
// glib.IdleAdd internally to marshal work onto the main loop.
func (s *Session) Open(ctx context.Context) error {
	log.Info().Str("relay", s.relayURL).Msg("Session.Open: parsing relay URL")
	relayURI, err := glib.UriParse(s.relayURL, glib.GUriFlagsNoneValue)
	if err != nil {
		return fmt.Errorf("parse relay URL: %w", err)
	}
	s.relayURI = relayURI

	log.Info().Str("network", s.networkHex).Msg("Session.Open: decoding network id")
	networkBytes, err := hexTo32(s.networkHex)
	if err != nil {
		return fmt.Errorf("network id: %w", err)
	}
	s.networkID = p2panda.NewNetworkIdFromData(networkBytes)

	log.Info().Str("topic", s.topicHex).Msg("Session.Open: decoding topic id")
	topicBytes, err := hexTo32(s.topicHex)
	if err != nil {
		return fmt.Errorf("topic id: %w", err)
	}
	s.topicID = p2panda.NewTopicIdFromData(topicBytes)

	log.Info().Msg("Session.Open: generating private key")
	s.privateKey = p2panda.NewPrivateKey()
	ownPub := s.privateKey.GetPublicKey()
	s.ownPubHex = ptrHex(ownPub.GetData(), PubkeySize)
	ownPub.Free()
	log.Info().Str("pubkey", s.ownPubHex).Msg("Session.Open: generated key pair")

	if s.bootstrap != "" {
		log.Info().Str("bootstrap", s.bootstrap).Msg("Session.Open: configuring bootstrap")
		bootBytes, err := hexTo32(s.bootstrap)
		if err != nil {
			return fmt.Errorf("bootstrap node id: %w", err)
		}
		b, err := p2panda.NewNodeIdFromData(bootBytes, relayURI)
		if err != nil {
			return fmt.Errorf("construct bootstrap node id: %w", err)
		}
		s.bootstrapN = b
	} else {
		log.Info().Msg("Session.Open: no bootstrap configured, relying on relay and mDNS")
	}

	log.Info().Msg("Session.Open: creating Node")
	s.node = p2panda.NewNode(
		s.privateKey,
		"sqlite::memory:",
		s.networkID,
		s.relayURI,
		s.bootstrapN,
		p2panda.MdnsDiscoveryModeActiveValue,
	)
	if s.node == nil {
		return errors.New("p2panda.NewNode returned nil")
	}

	nodeReady := make(chan error, 1)
	topicReady := make(chan error, 1)

	var onNodeSpawn gio.AsyncReadyCallback = func(_, resultPtr, _ uintptr) {
		log.Info().Msg("Session.Open: onNodeSpawn callback fired")
		if _, err := s.node.SpawnFinish(&gio.AsyncResultBase{Ptr: resultPtr}); err != nil {
			log.Error().Err(err).Msg("Session.Open: SpawnFinish(node) failed")
			nodeReady <- err
			return
		}
		log.Info().Msg("Session.Open: node spawned, creating topic")

		s.topic = p2panda.NewTopic(
			s.node,
			s.topicID,
			uint32(p2panda.TopicEphemeralValue|p2panda.TopicPersistentValue),
		)
		if s.topic == nil {
			log.Error().Msg("Session.Open: p2panda.NewTopic returned nil")
			nodeReady <- errors.New("NewTopic returned nil")
			return
		}

		s.connectSignals()

		var onTopicSpawn gio.AsyncReadyCallback = func(_, tpResultPtr, _ uintptr) {
			log.Info().Msg("Session.Open: onTopicSpawn callback fired")
			if _, err := s.topic.SpawnFinish(&gio.AsyncResultBase{Ptr: tpResultPtr}); err != nil {
				log.Error().Err(err).Msg("Session.Open: SpawnFinish(topic) failed")
				topicReady <- err
				return
			}
			log.Info().Msg("Session.Open: topic spawned successfully")
			topicReady <- nil
		}

		log.Info().Msg("Session.Open: calling topic.SpawnAsync")
		s.topic.SpawnAsync(nil, &onTopicSpawn, 0)
		nodeReady <- nil
	}

	log.Info().Msg("Session.Open: scheduling node.SpawnAsync on main loop")
	var kick glib.SourceFunc = func(uintptr) bool {
		log.Info().Msg("Session.Open: kick fired on main loop, calling node.SpawnAsync")
		s.node.SpawnAsync(nil, &onNodeSpawn, 0)
		return false
	}
	glib.IdleAdd(&kick, 0)

	log.Info().Msg("Session.Open: waiting for nodeReady")
	select {
	case err := <-nodeReady:
		if err != nil {
			return fmt.Errorf("spawn node: %w", err)
		}
		log.Info().Msg("Session.Open: nodeReady received")
	case <-ctx.Done():
		log.Warn().Msg("Session.Open: ctx cancelled while waiting for node")
		return ctx.Err()
	}

	log.Info().Msg("Session.Open: waiting for topicReady")
	select {
	case err := <-topicReady:
		if err != nil {
			return fmt.Errorf("spawn topic: %w", err)
		}
		log.Info().Msg("Session.Open: topicReady received")
	case <-ctx.Done():
		log.Warn().Msg("Session.Open: ctx cancelled while waiting for topic")
		return ctx.Err()
	}

	log.Info().Msg("Session.Open: complete")
	return nil
}

// connectSignals wires the topic's message/ephemeral-message signals into the
// Session's channels. Callbacks are closed over `s`, filter self-sent messages,
// and drop events if the consumer isn't keeping up (preferring recent state).
func (s *Session) connectSignals() {
	deliver := func(pkPtr, bPtr uintptr, ephemeral bool) {
		from := ptrHex((*p2panda.PublicKey)(unsafe.Pointer(pkPtr)).GetData(), PubkeySize)
		if from == s.ownPubHex {
			return
		}

		msg := (*glib.Bytes)(unsafe.Pointer(bPtr))
		var sz uint
		raw := unsafe.Slice((*byte)(unsafe.Pointer(msg.GetData(&sz))), sz)
		data := make([]byte, len(raw))
		copy(data, raw)

		s.mu.Lock()
		_, seen := s.seenPeer[from]
		if !seen {
			s.seenPeer[from] = struct{}{}
		}
		s.mu.Unlock()

		if !seen {
			select {
			case s.peers <- PeerEvent{Pubkey: from, Joined: true}:
			default:
			}
		}

		select {
		case s.messages <- Message{From: from, Data: data, Ephemeral: ephemeral}:
		case <-s.closed:
		}
	}

	onMsg := func(_ p2panda.Topic, pkPtr, _, bPtr uintptr) bool {
		deliver(pkPtr, bPtr, false)
		return true
	}
	s.onMsgCB = &onMsg
	s.topic.ConnectMessage(s.onMsgCB)

	onEph := func(_ p2panda.Topic, pkPtr, _, bPtr uintptr) {
		deliver(pkPtr, bPtr, true)
	}
	s.onEphCB = &onEph
	s.topic.ConnectEphemeralMessage(s.onEphCB)
}

// Publish enqueues a publish on the main loop and returns once the call
// completes (or fails). Safe to call from any goroutine.
func (s *Session) Publish(data []byte, ephemeral bool) error {
	if s.topic == nil {
		return errors.New("session not open")
	}

	done := make(chan error, 1)

	var kick glib.SourceFunc = func(uintptr) bool {
		bytes := glib.NewBytes(data, uint(len(data)))

		var onPub gio.AsyncReadyCallback = func(_, pResultPtr, _ uintptr) {
			if _, err := s.topic.PublishFinish(&gio.AsyncResultBase{Ptr: pResultPtr}); err != nil {
				done <- err
				return
			}
			done <- nil
		}

		s.topic.PublishAsync(bytes, ephemeral, nil, &onPub, 0)

		return false
	}
	glib.IdleAdd(&kick, 0)

	select {
	case err := <-done:
		return err
	case <-s.closed:
		return errors.New("session closed")
	}
}

// Close frees the native resources owned by the session. Safe to call multiple times.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)

		var kick glib.SourceFunc = func(uintptr) bool {
			if s.bootstrapN != nil {
				s.bootstrapN.Free()
				s.bootstrapN = nil
			}
			if s.topicID != nil {
				s.topicID.Free()
				s.topicID = nil
			}
			if s.networkID != nil {
				s.networkID.Free()
				s.networkID = nil
			}
			if s.privateKey != nil {
				s.privateKey.Free()
				s.privateKey = nil
			}
			return false
		}
		glib.IdleAdd(&kick, 0)
	})
}
