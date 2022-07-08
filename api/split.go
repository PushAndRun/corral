package api

// InputSplit contains the information about a contiguous chunk of an input file.
// startOffset and endOffset are inclusive. For example, if the startOffset was 10
// and the endOffset was 14, then the InputSplit would describe a 5 byte chunk
// of the file.
type InputSplit struct {
	Filename    string // The file that the input split operates on
	StartOffset int64  // The starting byte index of the split in the file
	EndOffset   int64  // The ending byte index (inclusive) of the split in the file
}

// Size returns the number of bytes that the InputSplit spans
func (i InputSplit) Size() int64 {
	return i.EndOffset - i.StartOffset + 1
}
