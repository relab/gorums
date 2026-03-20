package stream

// serverSequenceNumberBitMask is the high-bit partition used to distinguish
// server-initiated message IDs from client-initiated message IDs on a shared
// bidirectional stream.
const serverSequenceNumberBitMask = uint64(1) << 63

// ServerSequenceNumber marks seqNo as server-initiated.
func ServerSequenceNumber(seqNo uint64) uint64 {
	return seqNo | serverSequenceNumberBitMask
}

// isServerSequenceNumber reports whether seqNo belongs to the server-initiated
// message ID space.
func isServerSequenceNumber(seqNo uint64) bool {
	return seqNo&serverSequenceNumberBitMask != 0
}
