#!/bin/bash

export CGO_ENABLED=0 
export GOARCH=amd64 
export GOTRACEBACK=system 

go get gopkg.in/yaml.v2
go env -w GO111MODULE=auto

mkdir -p build/
echo -n "building build/checkup-linux  "
GOOS=linux go build -ldflags="-s -w" -a -o build/checkup-linux checkup.go && 
echo done

echo -n "building build/checkup-darwin  "
GOOS=darwin go build -ldflags="-s -w" -a -o build/checkup-darwin checkup.go && 
echo done