FROM golang:latest 

RUN go get github.com/dexm-coin/dexmd

ADD . /go/src/github.com/dexm-coin/dexmd
WORKDIR /go/src/github.com/dexm-coin/dexmd

RUN go build
RUN cp dexmd testnet/coordinator
WORKDIR /go/src/github.com/dexm-coin/dexmd/testnet/coordinator

RUN apt-get update && apt-get install -y python-pip
RUN pip install flask 

CMD ["python", "app.py"]