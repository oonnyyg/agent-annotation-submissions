package ogg

// crcTable is the CRC-32 table for the Ogg polynomial 0x04c11db7,
// non-reflected, as specified in RFC 3533.
var crcTable = func() [256]uint32 {
	var t [256]uint32
	for i := 0; i < 256; i++ {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if r&0x80000000 != 0 {
				r = (r << 1) ^ 0x04c11db7
			} else {
				r <<= 1
			}
		}
		t[i] = r
	}
	return t
}()

// crc32Ogg computes the checksum used by Ogg pages (init 0, no final xor).
func crc32Ogg(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ crcTable[byte(crc>>24)^b]
	}
	return crc
}
