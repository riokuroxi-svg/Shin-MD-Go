// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// dedup.go — huellas de contenido para no emitir dos veces seguidas el mismo
// mensaje. No toca IDs, versión, dispositivo ni ninguna metadata de WhatsApp.
package wa

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
)

func outboundFingerprint(chat JID, kind string, parts ...[]byte) string {
	h := sha256.New()
	writeFingerprintPart(h, []byte(chat.ToNonAD().String()))
	writeFingerprintPart(h, []byte(kind))
	for _, part := range parts {
		writeFingerprintPart(h, part)
	}
	return string(h.Sum(nil))
}

type fingerprintWriter interface{ Write([]byte) (int, error) }

func writeFingerprintPart(w fingerprintWriter, part []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(part)))
	_, _ = w.Write(size[:])
	_, _ = w.Write(part)
}

func (c *Client) queueDedup(ctx context.Context, chat JID, kind string, textLen int, priority bool, parts [][]byte, send func() error) error {
	key := outboundFingerprint(chat, kind, parts...)
	err := c.Queue.DoDedup(ctx, textLen, priority, key, send)
	if errors.Is(err, ErrDuplicateSuppressed) {
		return nil
	}
	return err
}

func bytesOf(parts ...string) [][]byte {
	out := make([][]byte, len(parts))
	for i, part := range parts {
		out[i] = []byte(part)
	}
	return out
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func stableJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("<json-error>")
	}
	return data
}
