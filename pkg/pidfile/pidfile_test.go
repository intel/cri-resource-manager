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
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	testPidFile = "pidfile-test.pid"
)

func prepare(t *testing.T) string {
	dir, err := mkTestDir(t)
	if err != nil {
		t.Errorf("failed to create test directory: %v", err)
		os.Exit(1)
	}

	SetPath(filepath.Join(dir, testPidFile))
	return dir
}

func TestDefaults(t *testing.T) {
	t.Run("TestDefaults", func(t *testing.T) {
		var (
			pid int
			err error
		)

		Remove()

		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())

		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())

		close()
		err = Write()
		require.NotNil(t, err)

		Remove()
		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())
	})
}

func TestGetSetPath(t *testing.T) {
	t.Run("TestTestGetSetPath", func(t *testing.T) {
		var (
			dir  string
			path string
		)

		dir = prepare(t)
		path = GetPath()
		require.Equal(t, path, filepath.Join(dir, testPidFile))
	})
}

func TestReadNonExisting(t *testing.T) {
	t.Run("TestReadNonExisting", func(t *testing.T) {
		var (
			pid int
			err error
		)

		prepare(t)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, 0)
	})
}

func TestRemoveNonExisting(t *testing.T) {
	t.Run("TestRemoveNonExisting", func(t *testing.T) {
		prepare(t)
		err := Remove()
		require.Nil(t, err)
	})
}

func TestRemoveExisting(t *testing.T) {
	t.Run("TestRemoveExisting", func(t *testing.T) {
		var (
			err error
		)

		prepare(t)
		err = Write()
		require.Nil(t, err)

		err = Remove()
		require.Nil(t, err)
	})
}

func TestWrite(t *testing.T) {
	t.Run("TestWrite", func(t *testing.T) {
		var (
			pid int
			err error
		)

		prepare(t)

		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())
	})
}

func TestReadClosed(t *testing.T) {
	t.Run("TestReadClosed", func(t *testing.T) {
		var (
			pid int
			err error
		)

		prepare(t)

		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())

		close()
		pid, err = Read()
		require.NotNil(t, err)
		require.Equal(t, pid, -1)
	})
}

func TestFailToOverwrite(t *testing.T) {
	t.Run("TestFailToOverwrite", func(t *testing.T) {
		var (
			pid int
			err error
		)

		prepare(t)

		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())

		close()
		err = Write()
		require.NotNil(t, err)
	})
}

func TestRemoveToOverwrite(t *testing.T) {
	t.Run("TestRemoveToOverwrite", func(t *testing.T) {
		var (
			pid int
			err error
		)

		prepare(t)

		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())

		err = Remove()
		require.Nil(t, err)

		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())
	})
}

func TestOwnerPid(t *testing.T) {
	t.Run("TestOwnerPid", func(t *testing.T) {
		var (
			pid int
			chk int
			err error
		)

		prepare(t)

		err = Write()
		require.Nil(t, err)

		pid, err = Read()
		require.Nil(t, err)
		require.Equal(t, pid, os.Getpid())

		chk, err = OwnerPid()
		require.Nil(t, err)
		require.Equal(t, pid, chk)
	})
}

func mkTestDir(t *testing.T) (string, error) {
	tmp, err := os.MkdirTemp("", ".pidfile-test*")
	if err != nil {
		return "", errors.Wrapf(err, "failed to create test directory")
	}

	t.Cleanup(func() {
		os.RemoveAll(tmp)
	})

	return tmp, nil
}
