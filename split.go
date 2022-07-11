package corral

import (
	"bufio"
	"github.com/ISE-SMILE/corral/api"
	humanize "github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
)

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func splitInputFile(file api.FileInfo, maxSplitSize int64) []api.InputSplit {
	splits := make([]api.InputSplit, 0)

	for startOffset := int64(0); startOffset < file.Size; startOffset += maxSplitSize {
		endOffset := min(startOffset+maxSplitSize-1, file.Size-1)
		newSplit := api.InputSplit{
			Filename:    file.Name,
			StartOffset: startOffset,
			EndOffset:   endOffset,
		}
		splits = append(splits, newSplit)
	}

	return splits
}

// inputBin is a collection of inputSplits.
type inputBin struct {
	splits []api.InputSplit
	// The total size of the inputBin. (The sum of the size of all splits)
	size int64
}

// packInputSplits partitions inputSplits into bins.
// The combined size of each bin will be no greater than maxBinSize
func packInputSplits(splits []api.InputSplit, maxBinSize int64) [][]api.InputSplit {
	if len(splits) == 0 {
		return [][]api.InputSplit{}
	}

	bins := make([]*inputBin, 1)
	bins[0] = &inputBin{
		splits: make([]api.InputSplit, 0),
		size:   0,
	}

	// Partition splits into bins using a naive Next-Fit packing algorithm
	for _, split := range splits {
		currBin := bins[len(bins)-1]

		if currBin.size+split.Size() <= maxBinSize {
			currBin.splits = append(currBin.splits, split)
			currBin.size += split.Size()
		} else {
			newBin := &inputBin{
				splits: []api.InputSplit{split},
				size:   split.Size(),
			}
			bins = append(bins, newBin)
		}
	}

	binnedSplits := make([][]api.InputSplit, len(bins))
	totalSize := int64(0)
	for i, bin := range bins {
		totalSize += bin.size
		binnedSplits[i] = bin.splits
	}
	log.Debugf("Average input bin size: %s", humanize.Bytes(uint64(totalSize/int64(len(bins)))))
	return binnedSplits
}

// countingSplitFunc wraps a bufio.SplitFunc and keeps track of the number of bytes advanced.
// Upon each scan, the value of *bytesRead will be incremented by the number of bytes
// that the SplitFunc advances.
func countingSplitFunc(split bufio.SplitFunc, bytesRead *int64) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		adv, tok, err := split(data, atEOF)
		(*bytesRead) += int64(adv)
		return adv, tok, err
	}
}
