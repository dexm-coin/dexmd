FROM golang:latest 

RUN go get github.com/dexm-coin/dexmd

ADD . /go/src/github.com/dexm-coin/dexmd
WORKDIR /go/src/github.com/dexm-coin/dexmd
RUN go build -o dexmd
RUN cp dexmd testnet/worker/dexmd

RUN cp dexmd testnet/worker/dexmd
WORKDIR /go/src/github.com/dexm-coin/dexmd/testnet/worker

RUN rm config.json
RUN touch config.json
RUN apt-get update && apt-get install -y python-pip
RUN pip install requests 
# RUN tc qdisc del dev eth0 root
# RUN tc qdisc add dev eth0 root netem delay 97ms
EXPOSE 3141:3141

CMD ["python", "app.py"]