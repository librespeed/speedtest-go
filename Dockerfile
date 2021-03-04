FROM golang:alpine AS build_base
#ENV GOARCH arm64
#ENV GOARCH amd64
RUN apk add --no-cache git gcc ca-certificates libc-dev \
&& mkdir -p /go/src/github.com/librespeed/ \
&& cd /go/src/github.com/librespeed/ \
&& git clone https://github.com/librespeed/speedtest-go.git
#RUN adduser -h /app/ -H -S speedtestuser
WORKDIR /go/src/github.com/librespeed/speedtest-go
RUN go get ./ && go build -ldflags "-w -s" -trimpath -o speedtest main.go

FROM alpine:3.9
RUN apk add ca-certificates

RUN adduser -h /app/ -H -S speedtestuser
WORKDIR /app
USER speedtestuser
COPY --from=build_base /go/src/github.com/librespeed/speedtest-go/speedtest .
COPY --from=build_base /go/src/github.com/librespeed/speedtest-go/assets ./assets
COPY --from=build_base /go/src/github.com/librespeed/speedtest-go/settings.toml .
#COPY --from=build_base /etc/passwd /etc/passwd

EXPOSE 8989

CMD ["./speedtest"]
