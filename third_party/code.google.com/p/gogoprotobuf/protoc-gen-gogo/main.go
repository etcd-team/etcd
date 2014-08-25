// Go support for Protocol Buffers - Google's data interchange format
//
// Copyright 2010 The Go Authors.  All rights reserved.
// http://code.google.com/p/goprotobuf/
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//     * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//     * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//     * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

/*
	A plugin for the Google protocol buffer compiler to generate Go code.

	This plugin takes no options and the protocol buffer file syntax does
	not yet define any options for Go, so program does no option evaluation.
	That may change.
*/

package main

import (
	"io/ioutil"
	"os"

	"github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/proto"
	"github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/protoc-gen-gogo/generator"

	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/defaultcheck"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/description"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/embedcheck"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/enumstringer"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/equal"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/face"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/gostring"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/marshalto"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/populate"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/size"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/stringer"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/union"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/unmarshal"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/unsafemarshaler"
	_ "github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/unsafeunmarshaler"

	"github.com/coreos/etcd/third_party/code.google.com/p/gogoprotobuf/plugin/testgen"

	"strings"
)

func main() {
	// Begin by allocating a generator. The request and response structures are stored there
	// so we can do error handling easily - the response structure contains the field to
	// report failure.
	g := generator.New()

	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		g.Error(err, "reading input")
	}

	if err := proto.Unmarshal(data, g.Request); err != nil {
		g.Error(err, "parsing input proto")
	}

	if len(g.Request.FileToGenerate) == 0 {
		g.Fail("no files to generate")
	}

	g.CommandLineParameters(g.Request.GetParameter())

	// Create a wrapped version of the Descriptors and EnumDescriptors that
	// point to the file that defines them.
	g.WrapTypes()

	g.SetPackageNames()
	g.BuildTypeNameMap()

	g.GenerateAllFiles()

	gtest := generator.New()

	if err := proto.Unmarshal(data, gtest.Request); err != nil {
		gtest.Error(err, "parsing input proto")
	}

	if len(gtest.Request.FileToGenerate) == 0 {
		gtest.Fail("no files to generate")
	}

	gtest.CommandLineParameters(gtest.Request.GetParameter())

	// Create a wrapped version of the Descriptors and EnumDescriptors that
	// point to the file that defines them.
	gtest.WrapTypes()

	gtest.SetPackageNames()
	gtest.BuildTypeNameMap()

	gtest.GeneratePlugin(testgen.NewPlugin())

	for i := 0; i < len(gtest.Response.File); i++ {
		if strings.Contains(*gtest.Response.File[i].Content, `//These tests are generated by code.google.com/p/gogoprotobuf/plugin/testgen`) {
			gtest.Response.File[i].Name = proto.String(strings.Replace(*gtest.Response.File[i].Name, ".pb.go", "pb_test.go", -1))
			g.Response.File = append(g.Response.File, gtest.Response.File[i])
		}
	}

	// Send back the results.
	data, err = proto.Marshal(g.Response)
	if err != nil {
		g.Error(err, "failed to marshal output proto")
	}
	_, err = os.Stdout.Write(data)
	if err != nil {
		g.Error(err, "failed to write output proto")
	}
}
