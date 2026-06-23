// SPDX-License-Identifier: GPL-3.0-only

package spec

// ChecksumAlgo identifies a supported hash algorithm.
type ChecksumAlgo uint8

const (
	AlgoSHA256 ChecksumAlgo = iota + 1
	AlgoSHA384
	AlgoSHA512
	AlgoSHA1
	AlgoMD5
)

func (a ChecksumAlgo) String() string {
	switch a {
	case AlgoSHA256:
		return "sha256"
	case AlgoSHA384:
		return "sha384"
	case AlgoSHA512:
		return "sha512"
	case AlgoSHA1:
		return "sha1"
	case AlgoMD5:
		return "md5"
	default:
		return "unknown"
	}
}

// Checksum is a parsed "algo:hex" checksum specification.
type Checksum struct {
	Algo ChecksumAlgo
	Hex  string
}

// String reconstructs the original "algo:hex" format.
func (c Checksum) String() string {
	return c.Algo.String() + ":" + c.Hex
}

// IsZero reports whether the checksum is unset.
func (c Checksum) IsZero() bool {
	return c.Algo == 0
}

// ParseChecksum parses an "algo:hex" string. Returns an error describing
// what went wrong on failure.
func ParseChecksum(s string) (Checksum, error) {
	i := indexByte(s, ':')
	if i < 0 || i == len(s)-1 {
		return Checksum{}, checksumParseError("must be \"algo:hex\"")
	}

	algo, hex := s[:i], s[i+1:]
	a, ok := parseAlgo(algo)
	if !ok {
		return Checksum{}, checksumParseError("unsupported algorithm \"" + algo + "\"")
	}

	return Checksum{Algo: a, Hex: hex}, nil
}

func parseAlgo(s string) (ChecksumAlgo, bool) {
	switch s {
	case "sha256":
		return AlgoSHA256, true
	case "sha384":
		return AlgoSHA384, true
	case "sha512":
		return AlgoSHA512, true
	case "sha1":
		return AlgoSHA1, true
	case "md5":
		return AlgoMD5, true
	default:
		return 0, false
	}
}

func indexByte(s string, c byte) int {
	for i := range len(s) {
		if s[i] == c {
			return i
		}
	}
	return -1
}

type checksumParseError string

func (e checksumParseError) Error() string { return string(e) }
