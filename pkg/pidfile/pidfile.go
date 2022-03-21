// Copyright 2022 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pidfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

var (
	pidFilePath = defaultPath()
	pidFile     *os.File
)

// GetPath returns the current pidfile path.
func GetPath() string {
	return pidFilePath
}

// SetPath sets the pidfile path to the given one.
func SetPath(path string) {
	close()
	pidFilePath = path
}

// Write opens the PID file and writes os.Getpid() to it. If the PID file already
// exists Write() fails with an error. On successful completion, Write keeps the
// PID file open.
func Write() error {
	if pidFile != nil {
		return nil
	}

	err := os.MkdirAll(filepath.Dir(pidFilePath), 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create PID file")
	}

	pidFile, err = os.OpenFile(pidFilePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create PID file")
	}

	_, err = pidFile.Write([]byte(fmt.Sprintf("%d\n", os.Getpid())))
	if err != nil {
		close()
		return errors.Wrap(err, "failed to write PID file")
	}

	return nil
}

// Read reads the content of the PID file. It returns the process ID found
// in the file. If opening the file or reading an integer process ID fails
// Read() returns -1 and an error.
func Read() (int, error) {
	var (
		pid int
		buf []byte
		err error
	)

	if buf, err = os.ReadFile(pidFilePath); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return -1, errors.Wrap(err, "failed to read PID file")
	}

	if pid, err = strconv.Atoi(strings.TrimRight(string(buf), "\n")); err != nil {
		return -1, errors.Wrapf(err, "invalid PID (%q) in PID file", string(buf))
	}

	return pid, nil
}

// close closes the PID file and truncates it to zero length.
func close() {
	if pidFile != nil {
		pidFile.Truncate(0)
		pidFile.Close()
		pidFile = nil
	}
}

// Remove removes the PID file for the process unconditionally, regardless if
// the current process had created the PID file or not.
func Remove() error {
	close()
	err := os.Remove(pidFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
	}
	return err
}

// OwnerPid returns the ID of the process owning the PID file. 0 is returned
// if it is known that no process owns the file. -1 and an error is returned
// if the owner or its existence could not be determined.
func OwnerPid() (int, error) {
	var (
		pid int
		p   *os.Process
		err error
	)

	pid, err = Read()
	if err != nil {
		return -1, err
	}
	if pid == 0 {
		return 0, nil
	}

	p, err = os.FindProcess(pid)
	if err != nil {
		return -1, errors.Wrapf(err, "FindProcess() failed for PID %d", pid)
	}

	err = p.Signal(syscall.Signal(0))
	if err == os.ErrProcessDone {
		return 0, nil
	}
	if err == nil {
		return pid, nil
	}

	return -1, errors.Wrapf(err, "failed to check process %d", pid)
}

// defaultPath returns the default pidfile path.
func defaultPath() string {
	var path string

	if len(os.Args) > 0 {
		name := filepath.Base(os.Args[0])
		if euid := os.Geteuid(); euid > 0 {
			path = filepath.Join("/tmp", name+".pid")
		} else {
			path = filepath.Join("/", "var", "run", name+".pid")
		}
	}

	return path
}
