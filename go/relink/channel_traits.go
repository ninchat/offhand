package relink

import (
	"encoding/binary"
	"io"
)

// Pre-defined implementations for common types.
//
// Template for custom byte array type (replace N with the size):
//
//     import (
//         "encoding/binary"
//         "io"
//
//         "github.com/ninchat/relink/go/relink"
//     )
//
//     var ByteNChannelTraits ChannelTraits = byteNChannelTraits{}
//
//     // byteNChannelTraits teaches Relink to handle [N]byte channel ids.
//     type byteNChannelTraits struct{}
//
//     func (byteNChannelTraits) IdSize() int {
//         return N
//     }
//
//     func (byteNChannelTraits) CreateMap() relink.ChannelMap {
//         return make(byteNChannelMap)
//     }
//
//     func (byteNChannelTraits) CreateAndReadIds(ids []relink.ChannelId, r io.Reader) (err error) {
//         buf := make([][N]byte, len(ids))
//
//         // binary.Read is used for convenience, no byteswapping is actually needed
//         if err = binary.Read(r, binary.LittleEndian, buf); err != nil {
//             return
//         }
//
//         for i, id := range buf {
//             ids[i] = id
//         }
//
//         return
//     }
//
//     // byteNChannelMap teaches Relink to index by [N]byte channel id.
//     type byteNChannelMap map[[N]byte]interface{}
//
//     func (m byteNChannelMap) Insert(id relink.ChannelId, x interface{}) {
//         m[id.([N]byte)] = x
//     }
//
//     func (m byteNChannelMap) Lookup(id relink.ChannelId) interface{} {
//         return m[id.([N]byte)]
//     }
//
//     func (m byteNChannelMap) Delete(id relink.ChannelId) {
//         delete(m, id.([N]byte))
//     }
//
//     func (m byteNChannelMap) Length() int {
//         return len(m)
//     }
//
//     func (m byteNChannelMap) Foreach(f func(ChannelId, interface{})) {
//         for id, x := range m {
//             f(id, x)
//         }
//     }
//
var (
	Int8ChannelTraits  ChannelTraits = int8ChannelTraits{}
	Int16ChannelTraits ChannelTraits = int16ChannelTraits{}
	Int32ChannelTraits ChannelTraits = int32ChannelTraits{}
	Int64ChannelTraits ChannelTraits = int64ChannelTraits{}

	Uint8ChannelTraits  ChannelTraits = uint8ChannelTraits{}
	Uint16ChannelTraits ChannelTraits = uint16ChannelTraits{}
	Uint32ChannelTraits ChannelTraits = uint32ChannelTraits{}
	Uint64ChannelTraits ChannelTraits = uint64ChannelTraits{}

	Byte2ChannelTraits  ChannelTraits = byte2ChannelTraits{}  // [2]byte
	Byte4ChannelTraits  ChannelTraits = byte4ChannelTraits{}  // [4]byte
	Byte8ChannelTraits  ChannelTraits = byte8ChannelTraits{}  // [8]byte
	Byte16ChannelTraits ChannelTraits = byte16ChannelTraits{} // [16]byte
)

type int8ChannelMap map[int8]interface{}
type int16ChannelMap map[int16]interface{}
type int32ChannelMap map[int32]interface{}
type int64ChannelMap map[int64]interface{}
type uint8ChannelMap map[uint8]interface{}
type uint16ChannelMap map[uint16]interface{}
type uint32ChannelMap map[uint32]interface{}
type uint64ChannelMap map[uint64]interface{}
type byte2ChannelMap map[[2]byte]interface{}
type byte4ChannelMap map[[4]byte]interface{}
type byte8ChannelMap map[[8]byte]interface{}
type byte16ChannelMap map[[16]byte]interface{}

func (m int8ChannelMap) Insert(id ChannelId, x interface{})   { m[id.(int8)] = x }
func (m int16ChannelMap) Insert(id ChannelId, x interface{})  { m[id.(int16)] = x }
func (m int32ChannelMap) Insert(id ChannelId, x interface{})  { m[id.(int32)] = x }
func (m int64ChannelMap) Insert(id ChannelId, x interface{})  { m[id.(int64)] = x }
func (m uint8ChannelMap) Insert(id ChannelId, x interface{})  { m[id.(uint8)] = x }
func (m uint16ChannelMap) Insert(id ChannelId, x interface{}) { m[id.(uint16)] = x }
func (m uint32ChannelMap) Insert(id ChannelId, x interface{}) { m[id.(uint32)] = x }
func (m uint64ChannelMap) Insert(id ChannelId, x interface{}) { m[id.(uint64)] = x }
func (m byte2ChannelMap) Insert(id ChannelId, x interface{})  { m[id.([2]byte)] = x }
func (m byte4ChannelMap) Insert(id ChannelId, x interface{})  { m[id.([4]byte)] = x }
func (m byte8ChannelMap) Insert(id ChannelId, x interface{})  { m[id.([8]byte)] = x }
func (m byte16ChannelMap) Insert(id ChannelId, x interface{}) { m[id.([16]byte)] = x }

func (m int8ChannelMap) Lookup(id ChannelId) interface{}   { return m[id.(int8)] }
func (m int16ChannelMap) Lookup(id ChannelId) interface{}  { return m[id.(int16)] }
func (m int32ChannelMap) Lookup(id ChannelId) interface{}  { return m[id.(int32)] }
func (m int64ChannelMap) Lookup(id ChannelId) interface{}  { return m[id.(int64)] }
func (m uint8ChannelMap) Lookup(id ChannelId) interface{}  { return m[id.(uint8)] }
func (m uint16ChannelMap) Lookup(id ChannelId) interface{} { return m[id.(uint16)] }
func (m uint32ChannelMap) Lookup(id ChannelId) interface{} { return m[id.(uint32)] }
func (m uint64ChannelMap) Lookup(id ChannelId) interface{} { return m[id.(uint64)] }
func (m byte2ChannelMap) Lookup(id ChannelId) interface{}  { return m[id.([2]byte)] }
func (m byte4ChannelMap) Lookup(id ChannelId) interface{}  { return m[id.([4]byte)] }
func (m byte8ChannelMap) Lookup(id ChannelId) interface{}  { return m[id.([8]byte)] }
func (m byte16ChannelMap) Lookup(id ChannelId) interface{} { return m[id.([16]byte)] }

func (m int8ChannelMap) Delete(id ChannelId)   { delete(m, id.(int8)) }
func (m int16ChannelMap) Delete(id ChannelId)  { delete(m, id.(int16)) }
func (m int32ChannelMap) Delete(id ChannelId)  { delete(m, id.(int32)) }
func (m int64ChannelMap) Delete(id ChannelId)  { delete(m, id.(int64)) }
func (m uint8ChannelMap) Delete(id ChannelId)  { delete(m, id.(uint8)) }
func (m uint16ChannelMap) Delete(id ChannelId) { delete(m, id.(uint16)) }
func (m uint32ChannelMap) Delete(id ChannelId) { delete(m, id.(uint32)) }
func (m uint64ChannelMap) Delete(id ChannelId) { delete(m, id.(uint64)) }
func (m byte2ChannelMap) Delete(id ChannelId)  { delete(m, id.([2]byte)) }
func (m byte4ChannelMap) Delete(id ChannelId)  { delete(m, id.([4]byte)) }
func (m byte8ChannelMap) Delete(id ChannelId)  { delete(m, id.([8]byte)) }
func (m byte16ChannelMap) Delete(id ChannelId) { delete(m, id.([16]byte)) }

func (m int8ChannelMap) Length() int   { return len(m) }
func (m int16ChannelMap) Length() int  { return len(m) }
func (m int32ChannelMap) Length() int  { return len(m) }
func (m int64ChannelMap) Length() int  { return len(m) }
func (m uint8ChannelMap) Length() int  { return len(m) }
func (m uint16ChannelMap) Length() int { return len(m) }
func (m uint32ChannelMap) Length() int { return len(m) }
func (m uint64ChannelMap) Length() int { return len(m) }
func (m byte2ChannelMap) Length() int  { return len(m) }
func (m byte4ChannelMap) Length() int  { return len(m) }
func (m byte8ChannelMap) Length() int  { return len(m) }
func (m byte16ChannelMap) Length() int { return len(m) }

func (m int8ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m int16ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m int32ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m int64ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m uint8ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m uint16ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m uint32ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m uint64ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m byte2ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m byte4ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m byte8ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

func (m byte16ChannelMap) Foreach(f func(ChannelId, interface{})) {
	for id, x := range m {
		f(id, x)
	}
}

type int8ChannelTraits struct{}
type int16ChannelTraits struct{}
type int32ChannelTraits struct{}
type int64ChannelTraits struct{}
type uint8ChannelTraits struct{}
type uint16ChannelTraits struct{}
type uint32ChannelTraits struct{}
type uint64ChannelTraits struct{}
type byte2ChannelTraits struct{}
type byte4ChannelTraits struct{}
type byte8ChannelTraits struct{}
type byte16ChannelTraits struct{}

func (int8ChannelTraits) IdSize() int   { return 1 }
func (int16ChannelTraits) IdSize() int  { return 2 }
func (int32ChannelTraits) IdSize() int  { return 4 }
func (int64ChannelTraits) IdSize() int  { return 8 }
func (uint8ChannelTraits) IdSize() int  { return 1 }
func (uint16ChannelTraits) IdSize() int { return 2 }
func (uint32ChannelTraits) IdSize() int { return 4 }
func (uint64ChannelTraits) IdSize() int { return 8 }
func (byte2ChannelTraits) IdSize() int  { return 2 }
func (byte4ChannelTraits) IdSize() int  { return 4 }
func (byte8ChannelTraits) IdSize() int  { return 8 }
func (byte16ChannelTraits) IdSize() int { return 16 }

func (int8ChannelTraits) CreateMap() ChannelMap   { return make(int8ChannelMap) }
func (int16ChannelTraits) CreateMap() ChannelMap  { return make(int16ChannelMap) }
func (int32ChannelTraits) CreateMap() ChannelMap  { return make(int32ChannelMap) }
func (int64ChannelTraits) CreateMap() ChannelMap  { return make(int64ChannelMap) }
func (uint8ChannelTraits) CreateMap() ChannelMap  { return make(uint8ChannelMap) }
func (uint16ChannelTraits) CreateMap() ChannelMap { return make(uint16ChannelMap) }
func (uint32ChannelTraits) CreateMap() ChannelMap { return make(uint32ChannelMap) }
func (uint64ChannelTraits) CreateMap() ChannelMap { return make(uint64ChannelMap) }
func (byte2ChannelTraits) CreateMap() ChannelMap  { return make(byte2ChannelMap) }
func (byte4ChannelTraits) CreateMap() ChannelMap  { return make(byte4ChannelMap) }
func (byte8ChannelTraits) CreateMap() ChannelMap  { return make(byte8ChannelMap) }
func (byte16ChannelTraits) CreateMap() ChannelMap { return make(byte16ChannelMap) }

func (int8ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([]int8, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (int16ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([]int16, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (int32ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([]int32, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (int64ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([]int64, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (uint8ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([]uint8, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (uint16ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([]uint16, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (uint32ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([]uint32, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (uint64ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([]uint64, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (byte2ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([][2]byte, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (byte4ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([][4]byte, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (byte8ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([][8]byte, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}

func (byte16ChannelTraits) CreateAndReadIds(ids []ChannelId, r io.Reader) (err error) {
	buf := make([][16]byte, len(ids))
	if err = binary.Read(r, binary.LittleEndian, buf); err == nil {
		for i, id := range buf {
			ids[i] = id
		}
	}
	return
}
