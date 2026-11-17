// Command machouuid rewrites the LC_UUID of Mach-O executables with a value derived from the file itself,
// so two builds of the same sources link to the same bytes (scripts/ffmpeg-build.sh, SDD-009).
//
// The linker derives the UUID from its inputs, and it varies from one link to the next; on arm64 the ad hoc
// code signature then hashes the changed pages too. This tool replaces the UUID with the first 16 bytes of
// the SHA-256 of the slice, taken with the 16 UUID bytes and the LC_CODE_SIGNATURE data zeroed, and marks
// them as a name based UUID (version 5, RFC 4122 variant), as ld does for its own digest. The caller signs
// again afterwards (codesign --force -s -), because the signature no longer matches the bytes.
//
// Usage: machouuid <file>...   (thin and fat files; every slice gets its own UUID)
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

const (
	magic64     = 0xfeedfacf
	fatMagic    = 0xcafebabe
	lcUUID      = 0x1b
	lcCodeSig   = 0x1d
	header64Len = 32
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: machouuid <file>...")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		if err := rewriteFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "machouuid: %s: %v\n", path, err)
			os.Exit(1)
		}
	}
}

func rewriteFile(path string) error {
	// The path is an argument of the FFmpeg build script, which names the binaries it just linked.
	info, err := os.Stat(path) //nolint:gosec // G703: a build tool given its own output paths
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path) //nolint:gosec // G703: see above
	if err != nil {
		return err
	}
	if err := rewrite(data); err != nil {
		return err
	}
	return os.WriteFile(path, data, info.Mode().Perm()) //nolint:gosec // G703: see above
}

// rewrite updates data in place; data is a thin 64-bit little endian Mach-O or a fat file of them.
func rewrite(data []byte) error {
	if len(data) < 4 {
		return errors.New("too short for a Mach-O file")
	}
	if binary.BigEndian.Uint32(data) == fatMagic {
		n := int(binary.BigEndian.Uint32(data[4:]))
		if n < 1 || 8+20*n > len(data) {
			return errors.New("bad fat header")
		}
		for i := range n {
			e := data[8+20*i:]
			off, size := uint64(binary.BigEndian.Uint32(e[8:])), uint64(binary.BigEndian.Uint32(e[12:]))
			if off+size > uint64(len(data)) {
				return errors.New("fat slice outside the file")
			}
			if err := rewriteThin(data[off : off+size]); err != nil {
				return fmt.Errorf("fat slice %d: %w", i, err)
			}
		}
		return nil
	}
	return rewriteThin(data)
}

func rewriteThin(s []byte) error {
	if len(s) < header64Len || binary.LittleEndian.Uint32(s) != magic64 {
		return errors.New("not a 64-bit little endian Mach-O slice")
	}
	ncmds := int(binary.LittleEndian.Uint32(s[16:]))
	sizeofcmds := int(binary.LittleEndian.Uint32(s[20:]))
	if header64Len+sizeofcmds > len(s) {
		return errors.New("load commands outside the file")
	}
	uuidAt, sigOff, sigSize := -1, 0, 0
	pos := header64Len
	for range ncmds {
		if pos+8 > header64Len+sizeofcmds {
			return errors.New("truncated load command")
		}
		cmd := binary.LittleEndian.Uint32(s[pos:])
		size := int(binary.LittleEndian.Uint32(s[pos+4:]))
		if size < 8 || pos+size > header64Len+sizeofcmds {
			return errors.New("bad load command size")
		}
		switch cmd {
		case lcUUID:
			if size < 24 {
				return errors.New("short LC_UUID")
			}
			uuidAt = pos + 8
		case lcCodeSig:
			if size < 16 {
				return errors.New("short LC_CODE_SIGNATURE")
			}
			sigOff = int(binary.LittleEndian.Uint32(s[pos+8:]))
			sigSize = int(binary.LittleEndian.Uint32(s[pos+12:]))
			if sigOff < 0 || sigSize < 0 || sigOff+sigSize > len(s) {
				return errors.New("code signature outside the file")
			}
		}
		pos += size
	}
	if uuidAt < 0 {
		return errors.New("no LC_UUID load command")
	}

	clear(s[uuidAt : uuidAt+16])
	var saved []byte
	if sigSize > 0 {
		saved = append(saved, s[sigOff:sigOff+sigSize]...)
		clear(s[sigOff : sigOff+sigSize])
	}
	sum := sha256.Sum256(s)
	if sigSize > 0 {
		copy(s[sigOff:], saved)
	}
	u := sum[:16]
	u[6] = u[6]&0x0f | 0x50
	u[8] = u[8]&0x3f | 0x80
	copy(s[uuidAt:], u)
	return nil
}
