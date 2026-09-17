package protocol

import (
	"crypto/cipher"
	"crypto/des"
	"encoding/binary"
	"fmt"
	"time"
)

// PumpTimestamp reproduces the vendor's milliseconds since local yesterday,
// treating that unspecified local date as UTC when subtracted from UtcNow.
func PumpTimestamp(now time.Time) uint32 {
	y, m, d := now.Date()
	origin := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	return uint32(now.UTC().Sub(origin).Milliseconds())
}

func PumpCommand(opcode byte, args []byte, timestamp uint32) ([]byte, error) {
	if len(args) > 492 {
		return nil, fmt.Errorf("pump command arguments exceed 492 bytes")
	}
	plain := make([]byte, 504)
	plain[0] = opcode
	plain[2] = 0x1a
	plain[3] = 0x6d
	binary.LittleEndian.PutUint32(plain[4:8], timestamp)
	copy(plain[8:500], args)
	copy(plain[500:], []byte{4, 4, 4, 4}) // DES PKCS7 padding of the 500-byte command.
	key := []byte("slv3tuzx")
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	packet := make([]byte, 512)
	cipher.NewCBCEncrypter(block, key).CryptBlocks(packet[:504], plain)
	packet[510] = 0xa1
	packet[511] = 0x1a
	return packet, nil
}

func PumpImage(payload []byte, png bool, timestamp uint32) ([]byte, error) {
	if len(payload) == 0 || len(payload) > 1048576 {
		return nil, fmt.Errorf("pump image payload must be 1..1048576 bytes")
	}
	opcode := byte(101)
	if png {
		opcode = 102
	}
	args := make([]byte, 4)
	binary.BigEndian.PutUint32(args, uint32(len(payload)))
	header, err := PumpCommand(opcode, args, timestamp)
	if err != nil {
		return nil, err
	}
	return append(header, payload...), nil
}
