FROM golang:1.21
COPY . /usr/src/server/
WORKDIR /usr/src/server/
RUN go env -w GOPROXY=direct
RUN go install ./main.go
CMD ["/go/bin/main"]
