// Package display sends one complete frame and never queues or retries a partial frame.
package display

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"time"

	"jonsbo-display/internal/imageutil"
	"jonsbo-display/internal/protocol"
)

type Transport interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	SetReadTimeout(time.Duration) error
}
type Result struct {
	BytesWritten int      `json:"bytes_written"`
	Replies      []string `json:"replies"`
	ElapsedMS    int64    `json:"elapsed_ms"`
	Status       string   `json:"status"`
}

func Dimensions(kind string) (int, int, error) {
	switch kind {
	case "pump":
		return 640, 480, nil
	case "fan":
		return 180, 640, nil
	default:
		return 0, 0, fmt.Errorf("unknown display kind %q", kind)
	}
}

func Send(ctx context.Context, t Transport, kind string, im image.Image) (result Result, err error) {
	start := time.Now()
	defer func() { result.ElapsedMS = time.Since(start).Milliseconds() }()
	if err = ctx.Err(); err != nil {
		return
	}
	w, h, e := Dimensions(kind)
	if e != nil {
		return result, e
	}
	if im == nil || im.Bounds().Dx() != w || im.Bounds().Dy() != h {
		return result, fmt.Errorf("%s requires %dx%d image", kind, w, h)
	}
	s := session{ctx: ctx, transport: t, result: &result}
	if kind == "fan" {
		err = s.fan(im)
	} else {
		err = s.pump(im)
	}
	if err == nil {
		result.Status = "USB transaction completed; physical display not yet confirmed"
	}
	return
}

type session struct {
	ctx       context.Context
	transport Transport
	result    *Result
}

func (s *session) write(p []byte) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	n, err := s.transport.Write(p)
	s.result.BytesWritten += n
	if err != nil {
		return err
	}
	if n != len(p) {
		return io.ErrShortWrite
	}
	return nil
}
func (s *session) read(size int) ([]byte, error) {
	// A 512-byte bulk reply may leave a zero-length USB terminator queued.
	// It is not an application response. Bound the drain to avoid spinning.
	for attempt := 0; attempt < 4; attempt++ {
		if err := s.ctx.Err(); err != nil {
			return nil, err
		}
		b := make([]byte, size)
		n, err := s.transport.Read(b)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}
		if n < 0 || n > size {
			return nil, io.ErrUnexpectedEOF
		}
		b = b[:n]
		s.result.Replies = append(s.result.Replies, hex.EncodeToString(b))
		return b, nil
	}
	return nil, io.ErrUnexpectedEOF
}
func (s *session) pause(d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *session) fanReply(expected byte) error {
	// A previous app may leave a frame or firmware reply queued. Consume at most
	// eight valid replies while waiting for the observed matching response.
	for i := 0; i < 8; i++ {
		raw, err := s.read(32)
		if err != nil {
			return err
		}
		r, err := protocol.DecodeFanReply(raw)
		if err != nil {
			return err
		}
		if r[2] == expected {
			return nil
		}
		if r[2] == 0x60 {
			return fmt.Errorf("fan rejected frame (status 0x60)")
		}
	}
	return fmt.Errorf("fan did not return expected status 0x%02x", expected)
}

func (s *session) fan(im image.Image) error {
	frame, err := protocol.FanFrame(imageutil.BGR(im))
	if err != nil {
		return err
	}
	if err = s.drainFan(); err != nil {
		return fmt.Errorf("fan receive queue: %w", err)
	}
	if err = s.write(protocol.FanCommand(0x38, 0)); err != nil {
		return fmt.Errorf("stop fan video: %w", err)
	}
	if err = s.pause(time.Millisecond); err != nil {
		return err
	}
	if err = s.write(protocol.FanCommand(0x58, 0)); err != nil {
		return fmt.Errorf("reset fan frame memory: %w", err)
	}
	if err = s.fanReply(0x59); err != nil {
		return fmt.Errorf("fan reset reply: %w", err)
	}
	if err = s.pause(200 * time.Millisecond); err != nil {
		return err
	}
	for offset := 0; offset < len(frame); offset += 4096 {
		if err = s.write(frame[offset : offset+4096]); err != nil {
			return fmt.Errorf("fan frame at byte %d: %w", offset, err)
		}
	}
	if err = s.pause(time.Millisecond); err != nil {
		return err
	}
	if err = s.fanReply(0x62); err != nil {
		return fmt.Errorf("fan frame reply: %w", err)
	}
	return nil
}

// Fan replies lack a transaction ID. Establish an idle receive boundary before
// writing new commands so queued same-opcode acknowledgements cannot match.
func (s *session) drainFan() (err error) {
	if err = s.transport.SetReadTimeout(100 * time.Millisecond); err != nil {
		return
	}
	defer func() { err = errors.Join(err, s.transport.SetReadTimeout(2*time.Second)) }()
	for i := 0; i < 16; i++ {
		if err = s.ctx.Err(); err != nil {
			return
		}
		var b [32]byte
		_, err = s.transport.Read(b[:])
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("receive queue did not become idle after 16 reads")
}

func (s *session) pumpExchange(p []byte, opcode byte, timestamp uint32) error {
	if err := s.write(p); err != nil {
		return err
	}
	for attempt := 0; attempt < 8; attempt++ {
		r, err := s.read(512)
		if err != nil {
			return err
		}
		if len(r) != 512 {
			return fmt.Errorf("pump response length %d, expected 512", len(r))
		}
		// Observed on this pump: byte 0 echoes opcode, byte 1 is C8 on
		// success, and bytes 2..5 echo the request timestamp little-endian.
		if r[0] != opcode || binary.LittleEndian.Uint32(r[2:6]) != timestamp {
			continue
		}
		if r[1] != 200 {
			return fmt.Errorf("pump command 0x%02x failed: status 0x%02x", opcode, r[1])
		}
		return nil
	}
	return fmt.Errorf("pump did not acknowledge command 0x%02x with matching timestamp", opcode)
}
func (s *session) pump(im image.Image) error {
	var clear, frame bytes.Buffer
	if err := png.Encode(&clear, image.NewNRGBA(image.Rect(0, 0, 640, 480))); err != nil {
		return err
	}
	if err := jpeg.Encode(&frame, im, &jpeg.Options{Quality: 95}); err != nil {
		return err
	}
	timestamp := protocol.PumpTimestamp(time.Now())
	mode, err := protocol.PumpCommand(41, []byte{0}, timestamp)
	if err != nil {
		return err
	}
	if err = s.pumpExchange(mode, 41, timestamp); err != nil {
		return fmt.Errorf("pump display mode: %w", err)
	}
	for i, payload := range [][]byte{clear.Bytes(), frame.Bytes()} {
		timestamp = protocol.PumpTimestamp(time.Now())
		packet, err := protocol.PumpImage(payload, i == 0, timestamp)
		if err != nil {
			return err
		}
		opcode := byte(101)
		if i == 0 {
			opcode = 102
		}
		if err = s.pumpExchange(packet, opcode, timestamp); err != nil {
			return fmt.Errorf("pump image stage %d: %w", i, err)
		}
	}
	return nil
}
