// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// activity.go — actividad visible y legítima: recibos de lectura y ciclo
// composing→paused usando exclusivamente las APIs públicas de whatsmeow.
package wa

import (
	"context"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	readBatchWindow = 180 * time.Millisecond
	typingRefresh   = 12 * time.Second
)

func readReceiptTarget(msg *IncomingMessage) (string, types.JID, types.JID, bool) {
	if msg == nil || msg.IsFromMe || msg.ID == "" || !msg.HasContent ||
		msg.Chat.Server == BroadcastServer || msg.Chat.User == "status" {
		return "", types.JID{}, types.JID{}, false
	}
	chat := msg.Chat.whatsmeow().ToNonAD()
	if chat.IsEmpty() {
		return "", types.JID{}, types.JID{}, false
	}
	var sender types.JID
	if msg.IsGroup || msg.Chat.Server == GroupServer {
		sender = msg.Sender.whatsmeow().ToNonAD()
		if sender.IsEmpty() {
			return "", types.JID{}, types.JID{}, false
		}
	}
	return chat.String() + "\x00" + sender.String(), chat, sender, true
}

// QueueReadReceipt agrupa recibos durante una ventana corta. La clave incluye
// chat Y remitente: WhatsApp permite agrupar varios IDs únicamente cuando los
// mandó la misma persona, algo especialmente importante en grupos.
func (c *Client) QueueReadReceipt(msg *IncomingMessage) {
	if c == nil || c.waClient == nil {
		return
	}
	key, chat, sender, ok := readReceiptTarget(msg)
	if !ok {
		return
	}
	timestamp := msg.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	c.readMu.Lock()
	if c.readPending == nil {
		c.readPending = make(map[string]*readBatch)
	}
	batch := c.readPending[key]
	if batch == nil {
		batch = &readBatch{chat: chat, sender: sender}
		c.readPending[key] = batch
	}
	id := types.MessageID(msg.ID)
	if len(batch.ids) == 0 || batch.ids[len(batch.ids)-1] != id {
		batch.ids = append(batch.ids, id)
	}
	if timestamp.After(batch.timestamp) {
		batch.timestamp = timestamp
	}
	if c.readTimer == nil {
		c.readTimer = time.AfterFunc(readBatchWindow, c.flushReadReceipts)
	}
	c.readMu.Unlock()
}

func (c *Client) flushReadReceipts() {
	c.readMu.Lock()
	pending := c.readPending
	c.readPending = make(map[string]*readBatch)
	c.readTimer = nil
	c.readMu.Unlock()

	base := c.baseCtx
	if base == nil {
		base = context.Background()
	}
	for _, batch := range pending {
		if batch == nil || len(batch.ids) == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(base, 8*time.Second)
		err := c.waClient.MarkRead(ctx, batch.ids, batch.timestamp, batch.chat, batch.sender)
		cancel()
		if err != nil && c.Log != nil && base.Err() == nil {
			c.Log.Warn("no se pudo marcar un chat como leído: " + err.Error())
		}
	}
}

// BeginComposing inicia el indicador de escritura y devuelve una función que
// SIEMPRE debe diferirse. Varias respuestas simultáneas en el mismo chat usan
// un contador: la primera en terminar no pausa el indicador de las demás.
func (c *Client) BeginComposing(ctx context.Context, chat JID) func() {
	if c == nil || (c.waClient == nil && c.presenceSender == nil) || chat.IsEmpty() ||
		chat.Server == BroadcastServer || chat.User == "status" {
		return func() {}
	}
	key := chat.ToNonAD().String()

	for {
		c.composingMu.Lock()
		if c.composing == nil {
			c.composing = make(map[string]*composingState)
		}
		if current := c.composing[key]; current != nil {
			if !current.closing {
				current.refs++
				c.composingMu.Unlock()
				return c.composingStopper(key, current, chat)
			}

			// El ciclo anterior ya se canceló, pero aún debe terminar su último
			// envío y mandar paused. Esperarlo evita que ese paused llegue después
			// del composing de una generación nueva del mismo chat.
			finished := current.finished
			c.composingMu.Unlock()
			if ctx == nil {
				<-finished
			} else {
				select {
				case <-finished:
				case <-ctx.Done():
					return func() {}
				}
			}
			continue
		}

		base := c.baseCtx
		if base == nil {
			base = ctx
		}
		if base == nil {
			base = context.Background()
		}
		if base.Err() != nil {
			c.composingMu.Unlock()
			return func() {}
		}
		stateCtx, cancel := context.WithCancel(base)
		state := &composingState{
			refs:     1,
			cancel:   cancel,
			done:     make(chan struct{}),
			finished: make(chan struct{}),
		}
		c.composing[key] = state
		c.composingMu.Unlock()

		go c.refreshComposing(stateCtx, chat, state.done)
		return c.composingStopper(key, state, chat)
	}
}

func (c *Client) composingStopper(key string, state *composingState, chat JID) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			c.composingMu.Lock()
			current := c.composing[key]
			if current != state || current.closing {
				c.composingMu.Unlock()
				return
			}
			current.refs--
			if current.refs > 0 {
				c.composingMu.Unlock()
				return
			}
			current.closing = true
			current.cancel()
			c.composingMu.Unlock()

			go func() {
				// Esperar al último composing garantiza el orden en red. El slot
				// permanece reservado hasta completar paused, por lo que una nueva
				// generación del chat tampoco puede adelantarse.
				<-state.done
				defer func() {
					c.composingMu.Lock()
					if c.composing[key] == state {
						delete(c.composing, key)
					}
					close(state.finished)
					c.composingMu.Unlock()
				}()
				c.sendChatPresence(c.baseCtx, chat, types.ChatPresencePaused)
			}()
		})
	}
}

func (c *Client) refreshComposing(ctx context.Context, chat JID, done chan<- struct{}) {
	defer close(done)
	c.sendChatPresence(ctx, chat, types.ChatPresenceComposing)
	ticker := time.NewTicker(typingRefresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendChatPresence(ctx, chat, types.ChatPresenceComposing)
		}
	}
}

func (c *Client) sendChatPresence(parent context.Context, chat JID, state types.ChatPresence) {
	if parent == nil {
		parent = context.Background()
	}
	if c.presenceSender != nil {
		c.presenceSender(parent, chat, state)
		return
	}
	if c.waClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	_ = c.waClient.SendChatPresence(ctx, chat.whatsmeow().ToNonAD(), state, types.ChatPresenceMediaText)
}
