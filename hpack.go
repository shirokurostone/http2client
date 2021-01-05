package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
)

func EncodeInteger(value int, n int) []byte {
	i := (1 << n) - 1
	if value < i {
		return []byte{byte(value)}
	}
	result := []byte{byte(i)}
	value -= i
	for value >= 128 {
		result = append(result, byte(value%128+128))
		value /= 128
	}
	result = append(result, byte(value))
	return result
}

func DecodeInteger(data []byte, n int) (int, int) {
	var v int

	i := (1 << n) - 1
	v = int(data[0] & byte(i))
	if v < i {
		return int(v), 1
	}

	v = 0
	j := 1
	m := 0
	for ; data[j]&0x80 == 0x80; j++ {
		v = v | (int(data[j]&0x7f) << m)
		m += 7
	}
	return v | (int(data[j]&0x7f) << m) + i, j
}

func EncodeHuffmanCode(str string, eos bool) []byte {
	b := []byte(str)
	result := []byte{}

	s := 0
	for i := 0; i < len(b); i++ {
		r := HuffmanTable[b[i]]

		for j := 0; j < r.BitLength; j++ {
			if s == 0 {
				result = append(result, 0)
			}
			result[len(result)-1] = result[len(result)-1]<<1 | byte((r.Code>>(r.BitLength-j-1))&1)
			s = (s + 1) % 8
		}
	}

	if eos {
		r := HuffmanTable[256]

		for j := 0; j < r.BitLength; j++ {
			if s == 0 {
				result = append(result, 0)
			}
			result[len(result)-1] = result[len(result)-1]<<1 | byte((r.Code>>(r.BitLength-j-1))&1)
			s = (s + 1) % 8
		}
	}

	result[len(result)-1] = result[len(result)-1]<<(8-s) | byte((1<<(8-s))-1)

	return result
}

func DecodeHuffmanCode(data []byte) string {
	result := []byte{}

	var p uint32 = 0
	s := 0
	for i := 0; i < len(data); i++ {
		for j := 0; j < 8; j++ {
			b := byte((data[i] >> (8 - j - 1)) & 0x01)
			p = p<<1 | uint32(b)
			s++

			for j := 0; j < 256; j++ {
				if HuffmanTable[j].Code == p && HuffmanTable[j].BitLength == s {
					result = append(result, byte(j))
					s = 0
					p = 0
				}
			}
		}

	}
	return string(result)
}

func EncodeHeaders(hl HeaderList) ([]byte, error) {
	result := make([]byte, 0)

	for i := 0; i < len(hl); i++ {
		h := hl[i]
		match := false
		for j := 1; j < len(StaticTable); j++ {
			if strings.Compare(h.Name, StaticTable[j].Name) == 0 {
				if strings.Compare(h.Value, StaticTable[j].Value) == 0 {
					result = append(result, h.DumpIndexedHeaderField(j)...)
					match = true
					break
				}
			}
		}
		if match {
			continue
		}
		for j := 1; j < len(StaticTable); j++ {
			if strings.Compare(h.Name, StaticTable[j].Name) == 0 {
				d, err := h.DumpLiteralHeaderFieldWithIndexedName(j, WITHOUT_INDEXING)
				if err != nil {
					return nil, err
				}
				result = append(result, d...)
				match = true
				break
			}
		}
		if match {
			continue
		}

		d, err := h.DumpLiteralHeaderFieldWithNewdName(WITHOUT_INDEXING)
		if err != nil {
			return nil, err
		}
		result = append(result, d...)
		match = true

	}
	return result, nil
}

func (d *HeaderDecoder) parseIndexedName(index int, input []byte, indexingType indexingType) (HeaderFieldFormat, int) {
	var result HeaderFieldFormat
	result.representationType = LITERAL_HEADER_FIELD
	result.indexingType = indexingType

	result.tableIndex = index

	field := d.getTableValue(index)
	if field == nil {
		log.Fatal("Header table error")
	}

	result.Name = (*field).Name

	i := 0
	valueLength, j := DecodeInteger(input[i:], 7)

	result.hValue = input[i]&0x80 == 0x80
	i += j
	if result.hValue {
		result.Value = DecodeHuffmanCode(input[i : i+valueLength])
	} else {
		result.Value = string(input[i : i+valueLength])
	}
	i += valueLength
	return result, i
}

func (d *HeaderDecoder) parseNewName(input []byte, indexingType indexingType) (HeaderFieldFormat, int) {
	var result HeaderFieldFormat
	result.representationType = LITERAL_HEADER_FIELD
	result.indexingType = indexingType

	i := 0
	nameLength, j := DecodeInteger(input[i:], 7)
	result.hName = input[i]&0x80 == 0x80
	i += j
	if result.hName {
		result.Name = DecodeHuffmanCode(input[i : i+nameLength])
	} else {
		result.Name = string(input[i : i+nameLength])
	}
	i += nameLength

	valueLength, j := DecodeInteger(input[i:], 7)

	result.hValue = input[i]&0x80 == 0x80
	i += j
	if result.hValue {
		result.Value = DecodeHuffmanCode(input[i : i+valueLength])
	} else {
		result.Value = string(input[i : i+valueLength])
	}
	i += valueLength

	return result, i
}

func (d *HeaderDecoder) parseHeaderBlockFragment(input []byte) []HeaderFieldFormat {
	var result []HeaderFieldFormat

	for i := 0; i < len(input); {

		b := input[i]

		switch {
		case b&0x80 == 0x80:
			// Indexed Header Field
			index, j := DecodeInteger(input[i:], 7)

			field := d.getTableValue(index)
			if field == nil {
				log.Fatal("Header table error")
			}

			f := HeaderFieldFormat{
				representationType: INDEX_HEADER_FIELD,
				tableIndex:         index,
				HeaderField:        *field,
			}

			result = append(result, f)
			i += j
			break
		case b&0xc0 == 0x40:
			// Literal Header Field with Incremental Indexing
			if b == 0x40 {
				// New Name
				i++
				f, j := d.parseNewName(input[i:], INCREMENTAL_INDEXING)
				result = append(result, f)
				d.insertIntoDynamicTable(HeaderField{f.Name, f.Value})
				i += j
			} else {
				// Indexed Name
				index, k := DecodeInteger(input[i:], 6)
				i += k
				f, j := d.parseIndexedName(index, input[i:], INCREMENTAL_INDEXING)
				result = append(result, f)
				d.insertIntoDynamicTable(HeaderField{f.Name, f.Value})
				i += j
			}
			break
		case b&0xf0 == 0x00:
			// Literal Header Field without Indexing
			if b == 0x00 {
				// New Name
				i++
				f, j := d.parseNewName(input[i:], WITHOUT_INDEXING)
				result = append(result, f)
				i += j
			} else {
				// Indexed Name
				index, k := DecodeInteger(input[i:], 4)
				i += k
				f, j := d.parseIndexedName(index, input[i:], WITHOUT_INDEXING)
				result = append(result, f)
				i += j
			}
			break
		case b&0xf0 == 0x10:
			// Literal Header Field Never Indexed
			if b == 0x10 {
				// New Name
				i++
				f, j := d.parseNewName(input[i:], NEVER_INDEXING)
				result = append(result, f)
				i += j
			} else {
				// Indexed Name
				index, k := DecodeInteger(input[i:], 4)
				i += k
				f, j := d.parseIndexedName(index, input[i:], NEVER_INDEXING)
				result = append(result, f)
				i += j
			}
			break
		case b&0xe0 == 0x20:
			// Maximum Dynamic Table Size Change
			max, k := DecodeInteger(input[i:], 5)
			i += k
			f := HeaderFieldFormat{
				representationType: DYNAMIC_TABLE_SIZE_UPDATE,
				MaxSize:            max,
			}
			result = append(result, f)
			d.MaxSize = max
			d.evictEntry()
			break
		default:
			// TODO: parse error
			break
		}

	}
	return result
}

type HeaderField struct {
	Name  string
	Value string
}

func (h *HeaderField) DumpIndexedHeaderField(index int) []byte {
	d := EncodeInteger(index, 7)
	d[0] |= 0x80
	return d
}

type indexingType byte

type representationType byte

type HeaderFieldFormat struct {
	representationType representationType
	indexingType       indexingType
	tableIndex         int
	hName              bool
	hValue             bool
	HeaderField
	MaxSize int
}

const (
	INDEX_HEADER_FIELD representationType = iota
	LITERAL_HEADER_FIELD
	DYNAMIC_TABLE_SIZE_UPDATE
)

const (
	INVALID_INDEXING indexingType = iota
	INCREMENTAL_INDEXING
	WITHOUT_INDEXING
	NEVER_INDEXING
)

func (h *HeaderField) DumpLiteralHeaderFieldWithIndexedName(index int, indexingType indexingType) ([]byte, error) {
	var result []byte
	switch indexingType {
	case INCREMENTAL_INDEXING:
		result = EncodeInteger(index, 6)
		result[0] |= 0x40
		break
	case WITHOUT_INDEXING:
		result = EncodeInteger(index, 4)
		break
	case NEVER_INDEXING:
		result = EncodeInteger(index, 4)
		result[0] |= 0x10
		break
	default:
		return nil, errIllegalArgument
	}

	d := EncodeInteger(len(h.Value), 7)
	result = append(result, d...)
	result = append(result, h.Value...)

	return result, nil
}

var errIllegalArgument = errors.New("IllegalArgument")

func (h *HeaderField) DumpLiteralHeaderFieldWithNewdName(indexingType indexingType) ([]byte, error) {
	var result []byte
	switch indexingType {
	case INCREMENTAL_INDEXING:
		result = []byte{0x40}
		break
	case WITHOUT_INDEXING:
		result = []byte{0x00}
		break
	case NEVER_INDEXING:
		result = []byte{0x10}
		break
	default:
		return nil, errIllegalArgument
	}

	result = append(result, EncodeInteger(len(h.Name), 7)...)
	result = append(result, h.Name...)

	result = append(result, EncodeInteger(len(h.Value), 7)...)
	result = append(result, h.Value...)

	return result, nil
}

type HeaderDecoder struct {
	DynamicTable []HeaderField
	MaxSize      int
}

func (d *HeaderDecoder) Decode(input []byte) map[string][]string {
	hl := d.parseHeaderBlockFragment(input)
	fmt.Printf("Headers: %#v\n", hl)

	headers := make(map[string][]string)
	for i := 0; i < len(hl); i++ {
		if hl[i].representationType != DYNAMIC_TABLE_SIZE_UPDATE {
			if v, ok := headers[hl[i].Name]; ok {
				headers[hl[i].Name] = append(v, hl[i].Value)
			} else {
				headers[hl[i].Name] = []string{hl[i].Value}
			}
		}
	}

	return headers
}

func (d *HeaderDecoder) getTableValue(index int) *HeaderField {
	if index < len(StaticTable) {
		return &StaticTable[index]
	} else if index-len(StaticTable) < len(d.DynamicTable) {
		return &d.DynamicTable[index-len(StaticTable)]
	}
	return nil
}

func (d *HeaderDecoder) insertIntoDynamicTable(field HeaderField) {
	d.DynamicTable = append(
		[]HeaderField{field},
		d.DynamicTable...)

	d.evictEntry()
}

func (d *HeaderDecoder) evictEntry() {
	for d.MaxSize < d.getDynamicTableSize() && len(d.DynamicTable) != 0 {
		d.DynamicTable = d.DynamicTable[0 : len(d.DynamicTable)-1]
	}
}

func (d *HeaderDecoder) getDynamicTableSize() int {
	size := 0
	for i := 0; i < len(d.DynamicTable); i++ {
		size += len(d.DynamicTable[i].Name) + len(d.DynamicTable[i].Value) + 32
	}
	return size
}

type HeaderList []HeaderField

var StaticTable []HeaderField = []HeaderField{
	{},
	{":authority", ""},
	{":method", "GET"},
	{":method", "POST"},
	{":path", "/"},
	{":path", "/index.html"},
	{":scheme", "http"},
	{":scheme", "https"},
	{":status", "200"},
	{":status", "204"},

	{":status", "206"},
	{":status", "304"},
	{":status", "400"},
	{":status", "404"},
	{":status", "500"},
	{":accept-charset", ""},
	{"accept-encoding", "gzip, deflate"},
	{"accept-language", ""},
	{"accept-ranges", ""},
	{"accept", ""},

	{"access-control-allow-origin", ""},
	{"age", ""},
	{"allow", ""},
	{"authorization", ""},
	{"cache-control", ""},
	{"content-disposition", ""},
	{"content-encoding", ""},
	{"content-language", ""},
	{"content-length", ""},
	{"content-location", ""},

	{"content-range", ""},
	{"content-type", ""},
	{"cookie", ""},
	{"date", ""},
	{"etag", ""},
	{"expect", ""},
	{"expires", ""},
	{"from", ""},
	{"host", ""},
	{"if-match", ""},

	{"if-modified-since", ""},
	{"if-none-match", ""},
	{"if-range ", ""},
	{"if-unmodified-since", ""},
	{"last-modified", ""},
	{"link", ""},
	{"location", ""},
	{"max-forwards", ""},
	{"proxy-authenticate", ""},
	{"proxy-authorization", ""},

	{"range", ""},
	{"referer", ""},
	{"refresh", ""},
	{"retry-after", ""},
	{"server", ""},
	{"set-cookie", ""},
	{"strict-transport-security", ""},
	{"transfer-encoding", ""},
	{"user-agent", ""},
	{"vary", ""},

	{"via", ""},
	{"www-authenticate", ""},
}

type HuffmanRecord struct {
	Code      uint32
	BitLength int
}

var HuffmanTable []HuffmanRecord = []HuffmanRecord{
	{0x1ff8, 13},
	{0x7fffd8, 23},
	{0xfffffe2, 28},
	{0xfffffe3, 28},
	{0xfffffe4, 28},
	{0xfffffe5, 28},
	{0xfffffe6, 28},
	{0xfffffe7, 28},
	{0xfffffe8, 28},
	{0xffffea, 24},
	{0x3ffffffc, 30},
	{0xfffffe9, 28},
	{0xfffffea, 28},
	{0x3ffffffd, 30},
	{0xfffffeb, 28},
	{0xfffffec, 28},
	{0xfffffed, 28},
	{0xfffffee, 28},
	{0xfffffef, 28},
	{0xffffff0, 28},
	{0xffffff1, 28},
	{0xffffff2, 28},
	{0x3ffffffe, 30},
	{0xffffff3, 28},
	{0xffffff4, 28},
	{0xffffff5, 28},
	{0xffffff6, 28},
	{0xffffff7, 28},
	{0xffffff8, 28},
	{0xffffff9, 28},
	{0xffffffa, 28},
	{0xffffffb, 28},
	{0x14, 6},
	{0x3f8, 10},
	{0x3f9, 10},
	{0xffa, 12},
	{0x1ff9, 13},
	{0x15, 6},
	{0xf8, 8},
	{0x7fa, 11},
	{0x3fa, 10},
	{0x3fb, 10},
	{0xf9, 8},
	{0x7fb, 11},
	{0xfa, 8},
	{0x16, 6},
	{0x17, 6},
	{0x18, 6},
	{0x0, 5},
	{0x1, 5},
	{0x2, 5},
	{0x19, 6},
	{0x1a, 6},
	{0x1b, 6},
	{0x1c, 6},
	{0x1d, 6},
	{0x1e, 6},
	{0x1f, 6},
	{0x5c, 7},
	{0xfb, 8},
	{0x7ffc, 15},
	{0x20, 6},
	{0xffb, 12},
	{0x3fc, 10},
	{0x1ffa, 13},
	{0x21, 6},
	{0x5d, 7},
	{0x5e, 7},
	{0x5f, 7},
	{0x60, 7},
	{0x61, 7},
	{0x62, 7},
	{0x63, 7},
	{0x64, 7},
	{0x65, 7},
	{0x66, 7},
	{0x67, 7},
	{0x68, 7},
	{0x69, 7},
	{0x6a, 7},
	{0x6b, 7},
	{0x6c, 7},
	{0x6d, 7},
	{0x6e, 7},
	{0x6f, 7},
	{0x70, 7},
	{0x71, 7},
	{0x72, 7},
	{0xfc, 8},
	{0x73, 7},
	{0xfd, 8},
	{0x1ffb, 13},
	{0x7fff0, 19},
	{0x1ffc, 13},
	{0x3ffc, 14},
	{0x22, 6},
	{0x7ffd, 15},
	{0x3, 5},
	{0x23, 6},
	{0x4, 5},
	{0x24, 6},
	{0x5, 5},
	{0x25, 6},
	{0x26, 6},
	{0x27, 6},
	{0x6, 5},
	{0x74, 7},
	{0x75, 7},
	{0x28, 6},
	{0x29, 6},
	{0x2a, 6},
	{0x7, 5},
	{0x2b, 6},
	{0x76, 7},
	{0x2c, 6},
	{0x8, 5},
	{0x9, 5},
	{0x2d, 6},
	{0x77, 7},
	{0x78, 7},
	{0x79, 7},
	{0x7a, 7},
	{0x7b, 7},
	{0x7ffe, 15},
	{0x7fc, 11},
	{0x3ffd, 14},
	{0x1ffd, 13},
	{0xffffffc, 28},
	{0xfffe6, 20},
	{0x3fffd2, 22},
	{0xfffe7, 20},
	{0xfffe8, 20},
	{0x3fffd3, 22},
	{0x3fffd4, 22},
	{0x3fffd5, 22},
	{0x7fffd9, 23},
	{0x3fffd6, 22},
	{0x7fffda, 23},
	{0x7fffdb, 23},
	{0x7fffdc, 23},
	{0x7fffdd, 23},
	{0x7fffde, 23},
	{0xffffeb, 24},
	{0x7fffdf, 23},
	{0xffffec, 24},
	{0xffffed, 24},
	{0x3fffd7, 22},
	{0x7fffe0, 23},
	{0xffffee, 24},
	{0x7fffe1, 23},
	{0x7fffe2, 23},
	{0x7fffe3, 23},
	{0x7fffe4, 23},
	{0x1fffdc, 21},
	{0x3fffd8, 22},
	{0x7fffe5, 23},
	{0x3fffd9, 22},
	{0x7fffe6, 23},
	{0x7fffe7, 23},
	{0xffffef, 24},
	{0x3fffda, 22},
	{0x1fffdd, 21},
	{0xfffe9, 20},
	{0x3fffdb, 22},
	{0x3fffdc, 22},
	{0x7fffe8, 23},
	{0x7fffe9, 23},
	{0x1fffde, 21},
	{0x7fffea, 23},
	{0x3fffdd, 22},
	{0x3fffde, 22},
	{0xfffff0, 24},
	{0x1fffdf, 21},
	{0x3fffdf, 22},
	{0x7fffeb, 23},
	{0x7fffec, 23},
	{0x1fffe0, 21},
	{0x1fffe1, 21},
	{0x3fffe0, 22},
	{0x1fffe2, 21},
	{0x7fffed, 23},
	{0x3fffe1, 22},
	{0x7fffee, 23},
	{0x7fffef, 23},
	{0xfffea, 20},
	{0x3fffe2, 22},
	{0x3fffe3, 22},
	{0x3fffe4, 22},
	{0x7ffff0, 23},
	{0x3fffe5, 22},
	{0x3fffe6, 22},
	{0x7ffff1, 23},
	{0x3ffffe0, 26},
	{0x3ffffe1, 26},
	{0xfffeb, 20},
	{0x7fff1, 19},
	{0x3fffe7, 22},
	{0x7ffff2, 23},
	{0x3fffe8, 22},
	{0x1ffffec, 25},
	{0x3ffffe2, 26},
	{0x3ffffe3, 26},
	{0x3ffffe4, 26},
	{0x7ffffde, 27},
	{0x7ffffdf, 27},
	{0x3ffffe5, 26},
	{0xfffff1, 24},
	{0x1ffffed, 25},
	{0x7fff2, 19},
	{0x1fffe3, 21},
	{0x3ffffe6, 26},
	{0x7ffffe0, 27},
	{0x7ffffe1, 27},
	{0x3ffffe7, 26},
	{0x7ffffe2, 27},
	{0xfffff2, 24},
	{0x1fffe4, 21},
	{0x1fffe5, 21},
	{0x3ffffe8, 26},
	{0x3ffffe9, 26},
	{0xffffffd, 28},
	{0x7ffffe3, 27},
	{0x7ffffe4, 27},
	{0x7ffffe5, 27},
	{0xfffec, 20},
	{0xfffff3, 24},
	{0xfffed, 20},
	{0x1fffe6, 21},
	{0x3fffe9, 22},
	{0x1fffe7, 21},
	{0x1fffe8, 21},
	{0x7ffff3, 23},
	{0x3fffea, 22},
	{0x3fffeb, 22},
	{0x1ffffee, 25},
	{0x1ffffef, 25},
	{0xfffff4, 24},
	{0xfffff5, 24},
	{0x3ffffea, 26},
	{0x7ffff4, 23},
	{0x3ffffeb, 26},
	{0x7ffffe6, 27},
	{0x3ffffec, 26},
	{0x3ffffed, 26},
	{0x7ffffe7, 27},
	{0x7ffffe8, 27},
	{0x7ffffe9, 27},
	{0x7ffffea, 27},
	{0x7ffffeb, 27},
	{0xffffffe, 28},
	{0x7ffffec, 27},
	{0x7ffffed, 27},
	{0x7ffffee, 27},
	{0x7ffffef, 27},
	{0x7fffff0, 27},
	{0x3ffffee, 26},
	{0x3fffffff, 30}, // 256: EOS
}
