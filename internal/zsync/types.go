package zsync

type ControlFile struct {
	URL       string
	Filename  string
	Length    int64
	SHA1      [20]byte
	BlockSize uint32
	HashLens  HashLengths
	Blocks    []BlockHash
}

type BlockHash struct {
	Weak   uint32
	Strong []byte
}

type HashLengths struct {
	SeqMatches uint8 // first value
	Weak       uint8 // middle value
	Strong     uint8 // last value
}
