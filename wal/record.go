/*
Copyright 2014 CoreOS Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package wal

import (
	"encoding/binary"
	"io"

	pb "github.com/coreos/etcd/wal/walpb"
)

func writeRecord(w io.Writer, rec *pb.Record) error {
	data, err := rec.Marshal()
	if err != nil {
		return err
	}

	if err := writeInt64(w, int64(len(data))); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func readRecord(r io.Reader, rec *pb.Record) error {
	rec.Reset()
	l, err := readInt64(r)
	if err != nil {
		return err
	}
	d := make([]byte, l)
	if _, err = io.ReadFull(r, d); err != nil {
		return err
	}
	return rec.Unmarshal(d)
}

func writeInt64(w io.Writer, n int64) error {
	return binary.Write(w, binary.LittleEndian, n)
}

func readInt64(r io.Reader) (int64, error) {
	var n int64
	err := binary.Read(r, binary.LittleEndian, &n)
	return n, err
}
