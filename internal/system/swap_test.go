package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSwapfilePathsFromFile(t *testing.T) {
	tests := []struct {
		name             string
		procSwapsContent string
		expectedSwaps    []*swap
		expectError      bool
		errorContains    string
	}{
		{
			name:             "no swap file exists",
			procSwapsContent: "",
			expectedSwaps:    []*swap{},
			expectError:      false,
		},
		{
			name: "single file swap",
			procSwapsContent: `Filename				Type		Size	Used	Priority
/swapfile                               file		2097148	0	-2
`,
			expectedSwaps: []*swap{
				{filePath: "/swapfile", swapType: "file"},
			},
			expectError: false,
		},
		{
			name: "single partition swap",
			procSwapsContent: `Filename				Type		Size	Used	Priority
/dev/sda2                               partition	1048572	0	-3
`,
			expectedSwaps: []*swap{
				{filePath: "/dev/sda2", swapType: "partition"},
			},
			expectError: false,
		},
		{
			name: "multiple swaps mixed types",
			procSwapsContent: `Filename				Type		Size	Used	Priority
/swapfile                               file		2097148	0	-2
/dev/sda2                               partition	1048572	0	-3
/var/swap                               file		524288	0	-4
`,
			expectedSwaps: []*swap{
				{filePath: "/swapfile", swapType: "file"},
				{filePath: "/dev/sda2", swapType: "partition"},
				{filePath: "/var/swap", swapType: "file"},
			},
			expectError: false,
		},
		{
			name: "header only no swaps",
			procSwapsContent: `Filename				Type		Size	Used	Priority
`,
			expectedSwaps: []*swap{},
			expectError:   false,
		},
		{
			name: "invalid format - too few fields",
			procSwapsContent: `Filename				Type		Size	Used	Priority
/swapfile file 2097148
`,
			expectedSwaps: nil,
			expectError:   true,
			errorContains: "has 3 fields, 5 are expected",
		},
		{
			name: "invalid format - too many fields",
			procSwapsContent: `Filename				Type		Size	Used	Priority
/swapfile file 2097148 0 -2 extra
`,
			expectedSwaps: nil,
			expectError:   true,
			errorContains: "has 6 fields, 5 are expected",
		},
		{
			name: "whitespace handling",
			procSwapsContent: `Filename				Type		Size	Used	Priority
  /swapfile  	file	2097148	0	-2  
`,
			expectedSwaps: []*swap{
				{filePath: "/swapfile", swapType: "file"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			procSwapsPath := filepath.Join(tempDir, "swaps")
			os.Setenv("ACTIVE_SWAP_AREAS", procSwapsPath)

			if tt.procSwapsContent != "" {
				err := os.WriteFile(procSwapsPath, []byte(tt.procSwapsContent), 0o644)
				require.NoError(t, err)
			}

			result, err := getSwapfilePaths()
			fmt.Println(result)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tt.expectedSwaps), len(result))

			for i, expected := range tt.expectedSwaps {
				assert.Equal(t, expected.filePath, result[i].filePath)
				assert.Equal(t, expected.swapType, result[i].swapType)
			}
		})
	}
}

func TestParseProcSwapsLine(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		expectedSwap  *swap
		expectError   bool
		errorContains string
	}{
		{
			name:         "valid file swap line",
			line:         "/swapfile                               file		2097148	0	-2",
			expectedSwap: &swap{filePath: "/swapfile", swapType: "file"},
			expectError:  false,
		},
		{
			name:         "valid partition swap line",
			line:         "/dev/sda2                               partition	1048572	0	-3",
			expectedSwap: &swap{filePath: "/dev/sda2", swapType: "partition"},
			expectError:  false,
		},
		{
			name:         "empty line",
			line:         "",
			expectedSwap: nil,
			expectError:  false,
		},
		{
			name:         "comment line",
			line:         "# This is a comment",
			expectedSwap: nil,
			expectError:  false,
		},
		{
			name:         "whitespace only line",
			line:         "   \t   ",
			expectedSwap: nil,
			expectError:  false,
		},
		{
			name:          "too few fields",
			line:          "/swapfile file 2097148",
			expectedSwap:  nil,
			expectError:   true,
			errorContains: "has 3 fields, 5 are expected",
		},
		{
			name:          "too many fields",
			line:          "/swapfile file 2097148 0 -2 extra",
			expectedSwap:  nil,
			expectError:   true,
			errorContains: "has 6 fields, 5 are expected",
		},
		{
			name:         "exactly 5 fields",
			line:         "/swapfile file 2097148 0 -2",
			expectedSwap: &swap{filePath: "/swapfile", swapType: "file"},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseProcSwapsLine(tt.line)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			assert.NoError(t, err)
			if tt.expectedSwap == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedSwap.filePath, result.filePath)
				assert.Equal(t, tt.expectedSwap.swapType, result.swapType)
			}
		})
	}
}

func TestPartitionSwapExistsLogic(t *testing.T) {
	tests := []struct {
		name           string
		swaps          []*swap
		expectedResult bool
	}{
		{
			name:           "no swaps",
			swaps:          []*swap{},
			expectedResult: false,
		},
		{
			name: "only file swap",
			swaps: []*swap{
				{filePath: "/swapfile", swapType: "file"},
			},
			expectedResult: false,
		},
		{
			name: "only partition swap",
			swaps: []*swap{
				{filePath: "/dev/sda2", swapType: "partition"},
			},
			expectedResult: true,
		},
		{
			name: "mixed swap types",
			swaps: []*swap{
				{filePath: "/swapfile", swapType: "file"},
				{filePath: "/dev/sda2", swapType: "partition"},
			},
			expectedResult: true,
		},
		{
			name: "multiple partition swaps",
			swaps: []*swap{
				{filePath: "/dev/sda2", swapType: "partition"},
				{filePath: "/dev/sdb1", swapType: "partition"},
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := false
			for _, swap := range tt.swaps {
				if swap.swapType == swapTypePartition {
					result = true
					break
				}
			}

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
