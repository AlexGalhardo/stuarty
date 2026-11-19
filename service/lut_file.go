package service

import (
	"encoding/binary"
	"os"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/lut"
)

// lutMagic and lutVersion start the binary table the preview reads (SDD-004 section 3.6).
const (
	lutMagic   = "VLUT"
	lutVersion = 1
)

// lutBody parses the normalized .cube artifact with the limits of engine/lut, so the browser never reads
// text, and returns it as the header (magic, version u8, size u16) followed by size^3 RGB entries of
// u16, little endian, the red index varying fastest.
func lutBody(resolve func(string) (string, bool), rel string) ([]byte, error) {
	path, ok := resolve(rel)
	if !ok {
		return nil, internalError()
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, newError(command.CodeNotFound, "the asset has no lut file.")
	}

	defer func() { _ = f.Close() }()

	t, err := lut.Parse(f)
	if err != nil {
		return nil, newError(command.CodeNotFound, "the lookup table cannot be read.")
	}

	body := make([]byte, 0, 7+2*len(t.Data))
	body = append(body, lutMagic...)
	body = append(body, lutVersion)
	body = binary.LittleEndian.AppendUint16(body, uint16(t.Size&0xFFFF))

	return append(body, t.Bytes()...), nil
}
