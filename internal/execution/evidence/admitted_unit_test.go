package evidence

import (
	"fmt"
	"strings"
	"testing"
)

// Partitioning writes must never change the retained bytes or short-write the
// command. Cover the boundary, partial prefix, suffix shift and large writes.
func TestAdmittedUnitBufferPartitions(t *testing.T) {
	for _, size := range []int{0, 1, StreamLimit/2 - 1, StreamLimit / 2, StreamLimit - 1, StreamLimit, StreamLimit + 1, 3*StreamLimit + 17} {
		input := strings.Repeat("0123456789abcdef", size/16+1)[:size]
		for _, chunk := range []int{1, 17, StreamLimit/2 - 1, StreamLimit / 2, StreamLimit, 2 * StreamLimit} {
			t.Run(fmt.Sprintf("size=%d/chunk=%d", size, chunk), func(t *testing.T) {
				var b Buffer
				for offset := 0; offset < len(input); {
					end := min(offset+chunk, len(input))
					n, err := b.Write([]byte(input[offset:end]))
					if err != nil || n != end-offset {
						t.Fatalf("short write: %d %v", n, err)
					}
					offset = end
				}
				if n, err := b.Write(nil); n != 0 || err != nil {
					t.Fatalf("empty write: %d %v", n, err)
				}
				want := input
				if size > StreamLimit {
					want = input[:StreamLimit/2] + input[size-StreamLimit/2:]
				}
				if b.String() != want || b.Len() != len(want) || b.Truncated != (size > StreamLimit) {
					t.Fatal("partition changed retained head/tail or truncation")
				}
			})
		}
	}
}
