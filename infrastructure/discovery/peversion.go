package discovery

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
)

// vsFixedFileInfoSignature starts the VS_FIXEDFILEINFO block of a version
// resource.
var vsFixedFileInfoSignature = []byte{0xBD, 0x04, 0xEF, 0xFE}

// peFileVersion returns the file version of a Windows executable, such as
// "3.0.1.24371", or "" when it has none. It works on any operating system.
func peFileVersion(path string) string {
	f, err := pe.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	section := f.Section(".rsrc")
	if section == nil {
		return ""
	}
	data, err := section.Data()
	if err != nil {
		return ""
	}
	return fixedFileVersion(data)
}

// fixedFileVersion reads dwFileVersionMS and dwFileVersionLS, which follow
// the signature and dwStrucVersion.
func fixedFileVersion(data []byte) string {
	i := bytes.Index(data, vsFixedFileInfoSignature)
	if i < 0 || len(data) < i+16 {
		return ""
	}
	ms := binary.LittleEndian.Uint32(data[i+8:])
	ls := binary.LittleEndian.Uint32(data[i+12:])
	return fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xffff, ls>>16, ls&0xffff)
}
