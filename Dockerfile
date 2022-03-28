FROM golang:1.18-alpine AS build
WORKDIR /go/src/github.com/utilitywarehouse/semaphore-wireguard
COPY . /go/src/github.com/utilitywarehouse/semaphore-wireguard
ENV CGO_ENABLED=0
RUN \
  apk --no-cache add git upx \
    && go get -t ./... \
    && go test -v ./... \
    && go build -ldflags='-s -w' -o /semaphore-wireguard . \
    && upx /semaphore-wireguard

FROM alpine:3.15
COPY --from=build /semaphore-wireguard /semaphore-wireguard
ENTRYPOINT [ "/semaphore-wireguard" ]
