FROM golang:latest 

RUN go get github.com/dexm-coin/dexmd

ADD . /go/src/github.com/dexm-coin/dexmd
WORKDIR /go/src/github.com/dexm-coin/dexmd

RUN go build -o main
RUN ["/go/src/github.com/dexm-coin/dexmd/main"]