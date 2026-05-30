package main

import "hash/crc32"

// mycrc32 returns the CRC32/IEEE checksum as uint32 — identical to Perl's implementation.
func mycrc32(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
