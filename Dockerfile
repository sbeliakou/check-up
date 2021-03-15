## docker build -t sbeliakou/ansible-lab-check:2.9.11-05 .
FROM golang

RUN  go get gopkg.in/yaml.v2

WORKDIR /build

COPY checkup.go /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOTRACEBACK=system go build -ldflags="-s -w" -a checkup.go 