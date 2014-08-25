#!/bin/bash

for filepath in `find . -type f ! -path "./third_party*" -name "*.proto"`; do
	protoc --gogo_out=. --proto_path=.:third_party:third_party/code.google.com/p/gogoprotobuf/protobuf/ $filepath
done

for filepath in `find . -type f ! -path "./third_party*" -name "*.pb.go"`; do
	sed -i '' 's/\"code.google.com\/p\/gogoprotobuf/\"github.com\/coreos\/etcd\/third_party\/code.google.com\/p\/gogoprotobuf/' $filepath
done
