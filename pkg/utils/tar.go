// Copyright 2019-2021 Intel Corporation. All Rights Reserved.
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

package utils

import (
	"archive/tar"
	"compress/bzip2"
	"io"
	"os"
	"path"
)

func UncompressTbz2(archive string, dir string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()

	data := bzip2.NewReader(file)
	tr := tar.NewReader(data)
	for {
		header, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if header.Typeflag == tar.TypeDir {
			// Create a directory.
			err = os.MkdirAll(path.Join(dir, header.Name), 0755)
			if err != nil {
				return err
			}
		} else if header.Typeflag == tar.TypeReg {
			// Create a regular file.
			targetFile, err := os.Create(path.Join(dir, header.Name))
			if err != nil {
				return err
			}
			_, err = io.Copy(targetFile, tr)
			targetFile.Close()
			if err != nil {
				return err
			}
		} else if header.Typeflag == tar.TypeSymlink {
			// Create a symlink and all the directories it needs.
			err = os.MkdirAll(path.Dir(path.Join(dir, header.Name)), 0755)
			if err != nil {
				return err
			}
			err := os.Symlink(header.Linkname, path.Join(dir, header.Name))
			if err != nil {
				return err
			}
		}
	}
}
