package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	containerMagic   = "HRTB"
	containerVersion = uint16(1)
	containerFlags   = uint16(0)
	containerHeader  = 16 // magic(4) + version(2) + flags(2) + payload length(8)
)

var (
	errWrongMagic    = errors.New("not an HRTB container")
	errWrongVersion  = errors.New("unsupported HRTB container version")
	errReservedFlags = errors.New("unsupported HRTB container flags")
	errTrailingBytes = errors.New("trailing bytes after HRTB payload")
	errTruncated     = errors.New("truncated HRTB container")
	errEmptyPayload  = errors.New("empty HRTB payload")
)

type container struct {
	Version uint16
	Flags   uint16
	Payload []byte
}

// makeContainer writes the experimental Phase 1F outer container:
//
//	magic "HRTB" | u16 version | u16 flags | u64 payload length | OpenPGP payload
//
// All integers are big-endian. The payload is one binary OpenPGP message.
func makeContainer(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errEmptyPayload
	}
	if len(payload) > maxContainerBytes-containerHeader {
		return nil, fmt.Errorf("%w: OpenPGP payload is %d bytes", errTooLarge, len(payload))
	}
	var buffer bytes.Buffer
	buffer.Grow(containerHeader + len(payload))
	buffer.WriteString(containerMagic)
	if err := binary.Write(&buffer, binary.BigEndian, containerVersion); err != nil {
		return nil, err
	}
	if err := binary.Write(&buffer, binary.BigEndian, containerFlags); err != nil {
		return nil, err
	}
	if err := binary.Write(&buffer, binary.BigEndian, uint64(len(payload))); err != nil {
		return nil, err
	}
	buffer.Write(payload)
	return buffer.Bytes(), nil
}

func parseContainer(raw []byte) (*container, error) {
	if len(raw) < containerHeader {
		return nil, fmt.Errorf("%w: header is %d bytes, need %d", errTruncated, len(raw), containerHeader)
	}
	if string(raw[:4]) != containerMagic {
		return nil, errWrongMagic
	}
	version := binary.BigEndian.Uint16(raw[4:6])
	flags := binary.BigEndian.Uint16(raw[6:8])
	length := binary.BigEndian.Uint64(raw[8:16])

	if version != containerVersion {
		return nil, fmt.Errorf("%w: %d", errWrongVersion, version)
	}
	if flags != containerFlags {
		return nil, fmt.Errorf("%w: %d", errReservedFlags, flags)
	}
	if length == 0 {
		return nil, errEmptyPayload
	}
	if length > maxContainerBytes-containerHeader {
		return nil, fmt.Errorf("%w: declared payload is %d bytes", errTooLarge, length)
	}
	actual := uint64(len(raw) - containerHeader)
	switch {
	case actual < length:
		return nil, fmt.Errorf("%w: declared %d payload bytes, got %d", errTruncated, length, actual)
	case actual > length:
		return nil, fmt.Errorf("%w: %d extra bytes", errTrailingBytes, actual-length)
	}
	return &container{
		Version: version,
		Flags:   flags,
		Payload: raw[containerHeader:],
	}, nil
}
