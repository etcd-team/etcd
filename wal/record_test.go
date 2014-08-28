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
	"bytes"
	"io"
	"reflect"
	"testing"

	pb "github.com/coreos/etcd/wal/walpb"
)

func TestReadRecord(t *testing.T) {
	tests := []struct {
		data []byte
		wr   *pb.Record
		we   error
	}{
		{infoRecord, &pb.Record{Type: 1, Crc: 0, Data: infoData}, nil},
		{[]byte(""), &pb.Record{}, io.EOF},
		{infoRecord[:len(infoRecord)-len(infoData)-8], &pb.Record{}, io.ErrUnexpectedEOF},
		{infoRecord[:len(infoRecord)-len(infoData)], &pb.Record{}, io.ErrUnexpectedEOF},
		{infoRecord[:len(infoRecord)-8], &pb.Record{}, io.ErrUnexpectedEOF},
	}

	rec := &pb.Record{}
	for i, tt := range tests {
		buf := bytes.NewBuffer(tt.data)
		e := readRecord(buf, rec)
		if !reflect.DeepEqual(rec, tt.wr) {
			t.Errorf("#%d: block = %v, want %v", i, rec, tt.wr)
		}
		if !reflect.DeepEqual(e, tt.we) {
			t.Errorf("#%d: err = %v, want %v", i, e, tt.we)
		}
		rec = &pb.Record{}
	}
}

func TestWriteRecord(t *testing.T) {
	b := &pb.Record{}
	typ := int64(0xABCD)
	d := []byte("Hello world!")
	buf := new(bytes.Buffer)
	writeRecord(buf, &pb.Record{Type: typ, Crc: 0, Data: d})
	err := readRecord(buf, b)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if b.Type != typ {
		t.Errorf("type = %d, want %d", b.Type, typ)
	}
	if !reflect.DeepEqual(b.Data, d) {
		t.Errorf("data = %v, want %v", b.Data, d)
	}
}
