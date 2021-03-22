FROM golang:1.16-alpine AS build
WORKDIR /go/src/github.com/utilitywarehouse/kube-wiresteward
COPY . /go/src/github.com/utilitywarehouse/kube-wiresteward
ENV CGO_ENABLED=0
RUN \
  apk --no-cache add git upx \
  && go get -t ./... \
  && go test -v \
  && go build -ldflags='-s -w' -o /kube-wiresteward . \
  && upx /kube-wiresteward

FROM alpine:3.13
COPY --from=build /kube-wiresteward /kube-wiresteward
ENTRYPOINT [ "/kube-wiresteward" ]
