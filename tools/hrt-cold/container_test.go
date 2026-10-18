package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestContainerRoundTrip(t *testing.T) {
	payload := []byte("synthetic OpenPGP payload")
	raw, err := makeContainer(payload)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseContainer(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Version != containerVersion || parsed.Flags != containerFlags {
		t.Fatalf("unexpected header: version=%d flags=%d", parsed.Version, parsed.Flags)
	}
	if !bytes.Equal(parsed.Payload, payload) {
		t.Fatal("payload mismatch")
	}
}

func TestContainerRejectsInvalidInput(t *testing.T) {
	valid, err := makeContainer([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	withUint16 := func(offset int, value uint16) []byte {
		raw := append([]byte(nil), valid...)
		binary.BigEndian.PutUint16(raw[offset:offset+2], value)
		return raw
	}
	withUint64 := func(offset int, value uint64) []byte {
		raw := append([]byte(nil), valid...)
		binary.BigEndian.PutUint64(raw[offset:offset+8], value)
		return raw
	}
	tests := []struct {
		name string
		in   []byte
		want error
	}{
		{"nil", nil, errTruncated},
		{"short header", []byte("HRTB"), errTruncated},
		{"bad magic", append([]byte("XXXX"), valid[4:]...), errWrongMagic},
		{"unknown version", withUint16(4, 2), errWrongVersion},
		{"reserved flags", withUint16(6, 1), errReservedFlags},
		{"zero length", withUint64(8, 0), errEmptyPayload},
		{"declared too large", withUint64(8, maxContainerBytes), errTooLarge},
		{"truncated payload", valid[:len(valid)-1], errTruncated},
		{"trailing bytes", append(append([]byte(nil), valid...), 0x00), errTrailingBytes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseContainer(test.in); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
	if _, err := makeContainer(nil); !errors.Is(err, errEmptyPayload) {
		t.Fatalf("empty payload: got %v", err)
	}
	if _, err := makeContainer(make([]byte, maxContainerBytes)); !errors.Is(err, errTooLarge) {
		t.Fatalf("oversized payload: got %v", err)
	}
}
