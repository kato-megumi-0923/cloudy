package wordlyrics

// QQ Music's lyrics come encrypted, in a Triple DES of its own: the S boxes
// carry typos, the key schedule drops the last bits, and the rounds stop at
// fifteen and a half. Standard libraries decrypt it to noise, so the cipher
// is here, ported from LDDC's tripledes.py, itself a port of the C# decoder
// QQ ships. Do not "fix" the tables: they are QQ's.

// qqSbox is QQ's eight substitution boxes.
var qqSbox = [8][64]byte{
	{
		14, 4, 13, 1, 2, 15, 11, 8, 3, 10, 6, 12, 5, 9, 0, 7,
		0, 15, 7, 4, 14, 2, 13, 1, 10, 6, 12, 11, 9, 5, 3, 8,
		4, 1, 14, 8, 13, 6, 2, 11, 15, 12, 9, 7, 3, 10, 5, 0,
		15, 12, 8, 2, 4, 9, 1, 7, 5, 11, 3, 14, 10, 0, 6, 13,
	},
	{
		15, 1, 8, 14, 6, 11, 3, 4, 9, 7, 2, 13, 12, 0, 5, 10,
		3, 13, 4, 7, 15, 2, 8, 15, 12, 0, 1, 10, 6, 9, 11, 5,
		0, 14, 7, 11, 10, 4, 13, 1, 5, 8, 12, 6, 9, 3, 2, 15,
		13, 8, 10, 1, 3, 15, 4, 2, 11, 6, 7, 12, 0, 5, 14, 9,
	},
	{
		10, 0, 9, 14, 6, 3, 15, 5, 1, 13, 12, 7, 11, 4, 2, 8,
		13, 7, 0, 9, 3, 4, 6, 10, 2, 8, 5, 14, 12, 11, 15, 1,
		13, 6, 4, 9, 8, 15, 3, 0, 11, 1, 2, 12, 5, 10, 14, 7,
		1, 10, 13, 0, 6, 9, 8, 7, 4, 15, 14, 3, 11, 5, 2, 12,
	},
	{
		7, 13, 14, 3, 0, 6, 9, 10, 1, 2, 8, 5, 11, 12, 4, 15,
		13, 8, 11, 5, 6, 15, 0, 3, 4, 7, 2, 12, 1, 10, 14, 9,
		10, 6, 9, 0, 12, 11, 7, 13, 15, 1, 3, 14, 5, 2, 8, 4,
		3, 15, 0, 6, 10, 10, 13, 8, 9, 4, 5, 11, 12, 7, 2, 14,
	},
	{
		2, 12, 4, 1, 7, 10, 11, 6, 8, 5, 3, 15, 13, 0, 14, 9,
		14, 11, 2, 12, 4, 7, 13, 1, 5, 0, 15, 10, 3, 9, 8, 6,
		4, 2, 1, 11, 10, 13, 7, 8, 15, 9, 12, 5, 6, 3, 0, 14,
		11, 8, 12, 7, 1, 14, 2, 13, 6, 15, 0, 9, 10, 4, 5, 3,
	},
	{
		12, 1, 10, 15, 9, 2, 6, 8, 0, 13, 3, 4, 14, 7, 5, 11,
		10, 15, 4, 2, 7, 12, 9, 5, 6, 1, 13, 14, 0, 11, 3, 8,
		9, 14, 15, 5, 2, 8, 12, 3, 7, 0, 4, 10, 1, 13, 11, 6,
		4, 3, 2, 12, 9, 5, 15, 10, 11, 14, 1, 7, 6, 0, 8, 13,
	},
	{
		4, 11, 2, 14, 15, 0, 8, 13, 3, 12, 9, 7, 5, 10, 6, 1,
		13, 0, 11, 7, 4, 9, 1, 10, 14, 3, 5, 12, 2, 15, 8, 6,
		1, 4, 11, 13, 12, 3, 7, 14, 10, 15, 6, 8, 0, 5, 9, 2,
		6, 11, 13, 8, 1, 4, 10, 7, 9, 5, 0, 15, 14, 2, 3, 12,
	},
	{
		13, 2, 8, 4, 6, 15, 11, 1, 10, 9, 3, 14, 5, 0, 12, 7,
		1, 15, 13, 8, 10, 3, 7, 4, 12, 5, 6, 11, 0, 14, 9, 2,
		7, 11, 4, 1, 9, 12, 14, 2, 0, 6, 10, 13, 15, 3, 5, 8,
		2, 1, 14, 7, 4, 10, 8, 13, 15, 12, 9, 0, 3, 5, 6, 11,
	},
}

// qqBit cuts bit b of data, shifted up by c.
func qqBit(data []byte, b, c int) uint32 {
	return uint32(data[(b/32)*4+3-(b%32)/8]>>(7-b%8)&1) << uint(c)
}

// qqBitIntr cuts bit b of a, counting from the top, shifted up by c.
func qqBitIntr(a uint32, b, c int) uint32 {
	return (a >> uint(31-b) & 1) << uint(c)
}

// qqBitIntl cuts bit b of a, counting from the bottom, shifted down by c.
func qqBitIntl(a uint32, b, c int) uint32 {
	return (a << uint(b) & 0x80000000) >> uint(c)
}

// qqSboxBit puts a six-bit index in QQ's order.
func qqSboxBit(a byte) int {
	return int(a&32 | (a&31)>>1 | (a&1)<<4)
}

// qqInitialPermutation spreads eight bytes over the two halves QQ starts
// its rounds with.
func qqInitialPermutation(data []byte) (uint32, uint32) {
	s0 := qqBit(data, 57, 31) | qqBit(data, 49, 30) | qqBit(data, 41, 29) | qqBit(data, 33, 28) |
		qqBit(data, 25, 27) | qqBit(data, 17, 26) | qqBit(data, 9, 25) | qqBit(data, 1, 24) |
		qqBit(data, 59, 23) | qqBit(data, 51, 22) | qqBit(data, 43, 21) | qqBit(data, 35, 20) |
		qqBit(data, 27, 19) | qqBit(data, 19, 18) | qqBit(data, 11, 17) | qqBit(data, 3, 16) |
		qqBit(data, 61, 15) | qqBit(data, 53, 14) | qqBit(data, 45, 13) | qqBit(data, 37, 12) |
		qqBit(data, 29, 11) | qqBit(data, 21, 10) | qqBit(data, 13, 9) | qqBit(data, 5, 8) |
		qqBit(data, 63, 7) | qqBit(data, 55, 6) | qqBit(data, 47, 5) | qqBit(data, 39, 4) |
		qqBit(data, 31, 3) | qqBit(data, 23, 2) | qqBit(data, 15, 1) | qqBit(data, 7, 0)
	s1 := qqBit(data, 56, 31) | qqBit(data, 48, 30) | qqBit(data, 40, 29) | qqBit(data, 32, 28) |
		qqBit(data, 24, 27) | qqBit(data, 16, 26) | qqBit(data, 8, 25) | qqBit(data, 0, 24) |
		qqBit(data, 58, 23) | qqBit(data, 50, 22) | qqBit(data, 42, 21) | qqBit(data, 34, 20) |
		qqBit(data, 26, 19) | qqBit(data, 18, 18) | qqBit(data, 10, 17) | qqBit(data, 2, 16) |
		qqBit(data, 60, 15) | qqBit(data, 52, 14) | qqBit(data, 44, 13) | qqBit(data, 36, 12) |
		qqBit(data, 28, 11) | qqBit(data, 20, 10) | qqBit(data, 12, 9) | qqBit(data, 4, 8) |
		qqBit(data, 62, 7) | qqBit(data, 54, 6) | qqBit(data, 46, 5) | qqBit(data, 38, 4) |
		qqBit(data, 30, 3) | qqBit(data, 22, 2) | qqBit(data, 14, 1) | qqBit(data, 6, 0)
	return s0, s1
}

// qqInversePermutation gathers the halves back into eight bytes.
func qqInversePermutation(s0, s1 uint32) []byte {
	data := make([]byte, 8)
	data[3] = byte(qqBitIntr(s1, 7, 7) | qqBitIntr(s0, 7, 6) | qqBitIntr(s1, 15, 5) |
		qqBitIntr(s0, 15, 4) | qqBitIntr(s1, 23, 3) | qqBitIntr(s0, 23, 2) |
		qqBitIntr(s1, 31, 1) | qqBitIntr(s0, 31, 0))
	data[2] = byte(qqBitIntr(s1, 6, 7) | qqBitIntr(s0, 6, 6) | qqBitIntr(s1, 14, 5) |
		qqBitIntr(s0, 14, 4) | qqBitIntr(s1, 22, 3) | qqBitIntr(s0, 22, 2) |
		qqBitIntr(s1, 30, 1) | qqBitIntr(s0, 30, 0))
	data[1] = byte(qqBitIntr(s1, 5, 7) | qqBitIntr(s0, 5, 6) | qqBitIntr(s1, 13, 5) |
		qqBitIntr(s0, 13, 4) | qqBitIntr(s1, 21, 3) | qqBitIntr(s0, 21, 2) |
		qqBitIntr(s1, 29, 1) | qqBitIntr(s0, 29, 0))
	data[0] = byte(qqBitIntr(s1, 4, 7) | qqBitIntr(s0, 4, 6) | qqBitIntr(s1, 12, 5) |
		qqBitIntr(s0, 12, 4) | qqBitIntr(s1, 20, 3) | qqBitIntr(s0, 20, 2) |
		qqBitIntr(s1, 28, 1) | qqBitIntr(s0, 28, 0))
	data[7] = byte(qqBitIntr(s1, 3, 7) | qqBitIntr(s0, 3, 6) | qqBitIntr(s1, 11, 5) |
		qqBitIntr(s0, 11, 4) | qqBitIntr(s1, 19, 3) | qqBitIntr(s0, 19, 2) |
		qqBitIntr(s1, 27, 1) | qqBitIntr(s0, 27, 0))
	data[6] = byte(qqBitIntr(s1, 2, 7) | qqBitIntr(s0, 2, 6) | qqBitIntr(s1, 10, 5) |
		qqBitIntr(s0, 10, 4) | qqBitIntr(s1, 18, 3) | qqBitIntr(s0, 18, 2) |
		qqBitIntr(s1, 26, 1) | qqBitIntr(s0, 26, 0))
	data[5] = byte(qqBitIntr(s1, 1, 7) | qqBitIntr(s0, 1, 6) | qqBitIntr(s1, 9, 5) |
		qqBitIntr(s0, 9, 4) | qqBitIntr(s1, 17, 3) | qqBitIntr(s0, 17, 2) |
		qqBitIntr(s1, 25, 1) | qqBitIntr(s0, 25, 0))
	data[4] = byte(qqBitIntr(s1, 0, 7) | qqBitIntr(s0, 0, 6) | qqBitIntr(s1, 8, 5) |
		qqBitIntr(s0, 8, 4) | qqBitIntr(s1, 16, 3) | qqBitIntr(s0, 16, 2) |
		qqBitIntr(s1, 24, 1) | qqBitIntr(s0, 24, 0))
	return data
}

// qqF is one round: the half and the round key mixed through QQ's boxes.
func qqF(state uint32, key [6]byte) uint32 {
	t1 := qqBitIntl(state, 31, 0) | (state&0xf0000000)>>1 | qqBitIntl(state, 4, 5) |
		qqBitIntl(state, 3, 6) | (state&0x0f000000)>>3 | qqBitIntl(state, 8, 11) |
		qqBitIntl(state, 7, 12) | (state&0x00f00000)>>5 | qqBitIntl(state, 12, 17) |
		qqBitIntl(state, 11, 18) | (state&0x000f0000)>>7 | qqBitIntl(state, 16, 23)
	t2 := qqBitIntl(state, 15, 0) | (state&0x0000f000)<<15 | qqBitIntl(state, 20, 5) |
		qqBitIntl(state, 19, 6) | (state&0x00000f00)<<13 | qqBitIntl(state, 24, 11) |
		qqBitIntl(state, 23, 12) | (state&0x000000f0)<<11 | qqBitIntl(state, 28, 17) |
		qqBitIntl(state, 27, 18) | (state&0x0000000f)<<9 | qqBitIntl(state, 0, 23)

	lg := [6]byte{
		byte(t1 >> 24), byte(t1 >> 16), byte(t1 >> 8),
		byte(t2 >> 24), byte(t2 >> 16), byte(t2 >> 8),
	}
	for i := range lg {
		lg[i] ^= key[i]
	}

	out := uint32(qqSbox[0][qqSboxBit(lg[0]>>2)])<<28 |
		uint32(qqSbox[1][qqSboxBit((lg[0]&0x03)<<4|lg[1]>>4)])<<24 |
		uint32(qqSbox[2][qqSboxBit((lg[1]&0x0f)<<2|lg[2]>>6)])<<20 |
		uint32(qqSbox[3][qqSboxBit(lg[2]&0x3f)])<<16 |
		uint32(qqSbox[4][qqSboxBit(lg[3]>>2)])<<12 |
		uint32(qqSbox[5][qqSboxBit((lg[3]&0x03)<<4|lg[4]>>4)])<<8 |
		uint32(qqSbox[6][qqSboxBit((lg[4]&0x0f)<<2|lg[5]>>6)])<<4 |
		uint32(qqSbox[7][qqSboxBit(lg[5]&0x3f)])

	return qqBitIntl(out, 15, 0) | qqBitIntl(out, 6, 1) | qqBitIntl(out, 19, 2) |
		qqBitIntl(out, 20, 3) | qqBitIntl(out, 28, 4) | qqBitIntl(out, 11, 5) |
		qqBitIntl(out, 27, 6) | qqBitIntl(out, 16, 7) | qqBitIntl(out, 0, 8) |
		qqBitIntl(out, 14, 9) | qqBitIntl(out, 22, 10) | qqBitIntl(out, 25, 11) |
		qqBitIntl(out, 4, 12) | qqBitIntl(out, 17, 13) | qqBitIntl(out, 30, 14) |
		qqBitIntl(out, 9, 15) | qqBitIntl(out, 1, 16) | qqBitIntl(out, 7, 17) |
		qqBitIntl(out, 23, 18) | qqBitIntl(out, 13, 19) | qqBitIntl(out, 31, 20) |
		qqBitIntl(out, 26, 21) | qqBitIntl(out, 2, 22) | qqBitIntl(out, 8, 23) |
		qqBitIntl(out, 18, 24) | qqBitIntl(out, 12, 25) | qqBitIntl(out, 29, 26) |
		qqBitIntl(out, 5, 27) | qqBitIntl(out, 21, 28) | qqBitIntl(out, 10, 29) |
		qqBitIntl(out, 3, 30) | qqBitIntl(out, 24, 31)
}

// qqCrypt runs the fifteen and a half rounds on eight bytes.
func qqCrypt(data []byte, key [16][6]byte) []byte {
	s0, s1 := qqInitialPermutation(data)
	for i := 0; i < 15; i++ {
		s1, s0 = qqF(s1, key[i])^s0, s1
	}
	s0 = qqF(s1, key[15]) ^ s0
	return qqInversePermutation(s0, s1)
}

// qqKeySchedule makes the sixteen round keys of one DES key.
func qqKeySchedule(key []byte, decrypt bool) [16][6]byte {
	var schedule [16][6]byte
	shifts := [16]int{1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1}
	permC := [28]int{56, 48, 40, 32, 24, 16, 8, 0, 57, 49, 41, 33, 25, 17, 9, 1, 58, 50, 42, 34, 26, 18, 10, 2, 59, 51, 43, 35}
	permD := [28]int{62, 54, 46, 38, 30, 22, 14, 6, 61, 53, 45, 37, 29, 21, 13, 5, 60, 52, 44, 36, 28, 20, 12, 4, 27, 19, 11, 3}
	compress := [48]int{13, 16, 10, 23, 0, 4, 2, 27, 14, 5, 20, 9, 22, 18, 11, 3, 25, 7, 15, 6, 26, 19, 12, 1, 40, 51, 30, 36,
		46, 54, 29, 39, 50, 44, 32, 47, 43, 48, 38, 55, 33, 52, 45, 41, 49, 35, 28, 31}

	var c, d uint32
	for i := 0; i < 28; i++ {
		c |= qqBit(key, permC[i], 31-i)
		d |= qqBit(key, permD[i], 31-i)
	}
	for i := 0; i < 16; i++ {
		shift := uint(shifts[i])
		c = (c<<shift | c>>(28-shift)) & 0xfffffff0
		d = (d<<shift | d>>(28-shift)) & 0xfffffff0
		at := i
		if decrypt {
			at = 15 - i
		}
		for j := 0; j < 24; j++ {
			schedule[at][j/8] |= byte(qqBitIntr(c, compress[j], 7-j%8))
		}
		for j := 24; j < 48; j++ {
			schedule[at][j/8] |= byte(qqBitIntr(d, compress[j]-27, 7-j%8))
		}
	}
	return schedule
}

// qqKeys turns the twenty-four byte key into the three the rounds use.
func qqKeys(key []byte, decrypt bool) [3][16][6]byte {
	if !decrypt {
		return [3][16][6]byte{qqKeySchedule(key[0:8], false), qqKeySchedule(key[8:16], true), qqKeySchedule(key[16:24], false)}
	}
	return [3][16][6]byte{qqKeySchedule(key[16:24], true), qqKeySchedule(key[8:16], false), qqKeySchedule(key[0:8], true)}
}

// qqTripleDES decrypts data block by block with QQ's three passes.
func qqTripleDES(data []byte, keys [3][16][6]byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i+8 <= len(data); i += 8 {
		block := data[i : i+8]
		for _, k := range keys {
			block = qqCrypt(block, k)
		}
		out = append(out, block...)
	}
	return out
}
