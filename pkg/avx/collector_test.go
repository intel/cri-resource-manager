package avx

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestGetHostKernelVersion(t *testing.T) {
	tcases := []struct {
		name          string
		procVersion   string
		errorExpected bool
		expectedMajor uint8
		expectedMinor uint8
		expectedPatch uint8
	}{
		{
			name:          "success",
			procVersion:   "Linux version 5.3.5-200.fc30.x86_64 (mockbuild@bkernel04.phx2.fedoraproject.org) (gcc version 9.2.1 20190827 (Red Hat 9.2.1-1) (GCC)) #1 SMP Tue Oct 8 12:41:15 UTC 2019",
			expectedMajor: 5,
			expectedMinor: 3,
			expectedPatch: 5,
		},
		{
			name:          "missing /proc/version",
			errorExpected: true,
		},
		{
			name:          "unparsable /proc/version",
			procVersion:   "unparsable fake content",
			errorExpected: true,
		},
	}

	for _, tt := range tcases {
		t.Run(tt.name, func(t *testing.T) {
			var tmpProcFile *os.File
			var procVersionPath string
			var err error

			if len(tt.procVersion) > 0 {
				tmpProcFile, err = ioutil.TempFile("", "avxcollectortest")
				if err != nil {
					t.Fatal("failed to create a temp file", err)
				}
				defer os.Remove(tmpProcFile.Name())
				tmpProcFile.WriteString(tt.procVersion)
				tmpProcFile.Close()
			}
			if tmpProcFile == nil {
				procVersionPath = "fakefile"
			} else {
				procVersionPath = tmpProcFile.Name()
			}
			major, minor, patch, err := getHostKernelVersion(procVersionPath)
			if err == nil && tt.errorExpected {
				t.Error("unexpected success")
			}
			if err != nil && !tt.errorExpected {
				t.Errorf("unexpected failure %+v", err)
			}
			if major != tt.expectedMajor {
				t.Errorf("expected major %d, but got %d", tt.expectedMajor, major)
			}
			if minor != tt.expectedMinor {
				t.Errorf("expected minor %d, but got %d", tt.expectedMinor, minor)
			}
			if patch != tt.expectedPatch {
				t.Errorf("expected patch %d, but got %d", tt.expectedPatch, patch)
			}
		})
	}
}
