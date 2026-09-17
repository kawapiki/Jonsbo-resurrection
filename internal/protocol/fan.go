package protocol

import (
	"crypto/des"
	"encoding/binary"
	"fmt"
)

var fanKey = []byte{0x41, 0x5f, 0xd9, 0xfa, 0x13, 0x42, 0x58, 0xb7}

func FanCommand(opcode, arg byte) []byte {
	packet := make([]byte, 32)
	packet[0] = 0xaa
	packet[1] = 0x55
	packet[10] = 1
	packet[31] = 0xbb
	block, _ := des.NewCipher(fanKey)
	block.Encrypt(packet[2:10], []byte{opcode, arg, 0, 0, 0, 0, 0, 0})
	return packet
}

func DecodeFanReply(packet []byte) ([]byte, error) {
	if len(packet) != 32 || packet[0] != 0xcc || packet[1] != 0x55 || packet[31] != 0xdd {
		return nil, fmt.Errorf("invalid fan response: %d bytes", len(packet))
	}
	out := append([]byte(nil), packet...)
	block, _ := des.NewCipher(fanKey)
	block.Decrypt(out[2:10], packet[2:10])
	return out, nil
}

func FanFrame(pixels []byte) ([]byte, error) {
	if len(pixels) != 180*640*3 {
		return nil, fmt.Errorf("fan requires exactly 180x640 BGR24 pixels")
	}
	packet := make([]byte, 720*512)
	for i := 0; i < 720; i++ {
		record := packet[i*512 : (i+1)*512]
		record[0] = 0xaa
		record[1] = 0x55
		record[10] = 2
		binary.BigEndian.PutUint16(record[11:13], uint16(i+1))
		record[31] = 0xbb
		copy(record[32:], pixels[i*480:(i+1)*480])
	}
	return packet, nil
}
