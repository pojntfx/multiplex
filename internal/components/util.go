package components

import (
	"unsafe"

	"github.com/jwijenbergh/puregotk/v4/gobject"
)

// typeQuery is a workaround for puregotk's gobject.TypeQuery struct having
// incorrect field types. The upstream struct uses Go's uint (64-bit on 64-bit systems)
// for ClassSize and InstanceSize, but C's guint is always 32-bit. This causes struct
// misalignment where C writes at offset 20 but Go reads at offset 24, resulting in
// InstanceSize being read as 0.
// See https://news.ycombinator.com/item?id=18964485
// TODO: We should further investigate this and then fix puregotk upstream
type typeQuery struct {
	Type         gobject.Type
	TypeName     uintptr
	ClassSize    uint32
	InstanceSize uint32
}

func newTypeQuery(gType gobject.Type) typeQuery {
	var buf [24]byte // Type (8 bytes) + TypeName (8 bytes) + ClassSize (4 bytes) + InstanceSize (4 bytes)
	gobject.NewTypeQuery(gType, (*gobject.TypeQuery)(unsafe.Pointer(&buf)))
	return *(*typeQuery)(unsafe.Pointer(&buf))
}
