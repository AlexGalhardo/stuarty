package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// fixture builds a thin Mach-O slice: header, LC_UUID, optionally LC_CODE_SIGNATURE, then a body and the signature.
func fixture(uuid [16]byte, body string, sig string) []byte {
	ncmds, cmds := 1, 24
	if sig != "" {
		ncmds, cmds = 2, 24+16
	}
	b := make([]byte, header64Len+cmds)
	binary.LittleEndian.PutUint32(b[0:], magic64)
	binary.LittleEndian.PutUint32(b[16:], uint32(ncmds))
	binary.LittleEndian.PutUint32(b[20:], uint32(cmds))
	binary.LittleEndian.PutUint32(b[32:], lcUUID)
	binary.LittleEndian.PutUint32(b[36:], 24)
	copy(b[40:], uuid[:])
	b = append(b, body...)
	if sig != "" {
		binary.LittleEndian.PutUint32(b[56:], lcCodeSig)
		binary.LittleEndian.PutUint32(b[60:], 16)
		binary.LittleEndian.PutUint32(b[64:], uint32(len(b)))
		binary.LittleEndian.PutUint32(b[68:], uint32(len(sig)))
		b = append(b, sig...)
	}
	return b
}

func TestUUIDIgnoresLinkerUUIDAndSignature(t *testing.T) {
	a := fixture([16]byte{1, 2, 3}, "same body", "signature one")
	b := fixture([16]byte{9, 9, 9}, "same body", "another sig!!")
	if err := rewrite(a); err != nil {
		t.Fatal(err)
	}
	if err := rewrite(b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a[40:56], b[40:56]) {
		t.Fatalf("uuid differs: %x vs %x", a[40:56], b[40:56])
	}
	if bytes.Equal(a[40:56], make([]byte, 16)) {
		t.Fatal("uuid is zero")
	}
	if a[46]>>4 != 5 || a[48]>>6 != 2 {
		t.Fatalf("version or variant not set: %x", a[40:56])
	}
	if !bytes.HasSuffix(a, []byte("signature one")) {
		t.Fatal("signature bytes were not restored")
	}
}

func TestUUIDFollowsTheBody(t *testing.T) {
	a := fixture([16]byte{}, "body one", "")
	b := fixture([16]byte{}, "body two", "")
	if err := rewrite(a); err != nil {
		t.Fatal(err)
	}
	if err := rewrite(b); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a[40:56], b[40:56]) {
		t.Fatal("different bodies gave the same uuid")
	}
	c := fixture([16]byte{}, "body one", "")
	if err := rewrite(c); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, c) {
		t.Fatal("not deterministic")
	}
}

func TestFatSlices(t *testing.T) {
	s := fixture([16]byte{7}, "slice", "")
	data := make([]byte, 8+20)
	binary.BigEndian.PutUint32(data, fatMagic)
	binary.BigEndian.PutUint32(data[4:], 1)
	binary.BigEndian.PutUint32(data[16:], 28)
	binary.BigEndian.PutUint32(data[20:], uint32(len(s)))
	data = append(data, s...)
	if err := rewrite(data); err != nil {
		t.Fatal(err)
	}
	want := fixture([16]byte{7}, "slice", "")
	if err := rewrite(want); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data[28:], want) {
		t.Fatal("fat slice differs from the thin result")
	}
}

func TestRejectsOtherFiles(t *testing.T) {
	if err := rewrite([]byte("not a macho file at all, really not....")); err == nil {
		t.Fatal("expected an error")
	}
	noUUID := fixture([16]byte{}, "x", "")
	binary.LittleEndian.PutUint32(noUUID[32:], 0x2)
	if err := rewrite(noUUID); err == nil {
		t.Fatal("expected an error without LC_UUID")
	}
}
