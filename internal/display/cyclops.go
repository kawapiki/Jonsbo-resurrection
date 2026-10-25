package display

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
)

// Cyclops uses a class SET_REPORT control request and an OUT-only pixel stream.
// The protocol is from the official RJK CYCLOPS 1.3.6 application; see docs.
type controlTransport interface{ Control([]byte) (int, error) }

func (s *session) cyclops(im image.Image) error {
	t, ok := s.transport.(controlTransport)
	if !ok {
		return fmt.Errorf("Cyclops requires USB control transfers")
	}
	frame := make([]byte, 115712) // 115200 RGB565 bytes + 512 padding, 113 x 1024.
	bounds := im.Bounds()
	for y := 0; y < 240; y++ {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		for x := 0; x < 240; x++ {
			r, g, b, _ := im.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			pixel := uint16((r>>11)<<11 | (g>>10)<<5 | b>>11)
			binary.LittleEndian.PutUint16(frame[(y*240+x)*2:], pixel)
		}
	}
	// Software applies mounting rotation. Reset the controller to 0 degrees and
	// use the vendor default brightness (4 of 8) so old settings cannot rotate twice.
	for _, command := range [][]byte{{0x81, 4, 0, 4, 0, 0, 0, 0x76}, {0x81, 1, 0, 0, 0, 0, 0, 0x7d}} {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		n, err := t.Control(command)
		s.result.BytesWritten += n
		if err != nil {
			return fmt.Errorf("Cyclops display control: %w", err)
		}
		if n != len(command) {
			return io.ErrShortWrite
		}
	}
	for offset := 0; offset < len(frame); offset += 1024 {
		if err := s.write(frame[offset : offset+1024]); err != nil {
			return fmt.Errorf("Cyclops frame at byte %d: %w", offset, err)
		}
	}
	// Vendor sends no IN request and defines no frame acknowledgement.
	return nil
}
