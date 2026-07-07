FROM golang:1.21-bullseye AS gobuild
# Go module proxy. Defaults to the public proxy; override for faster builds
# behind a slow network, e.g.:
#   docker build --build-arg GOPROXY=https://goproxy.cn,direct .
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
ADD . /device-plugin
RUN cd /device-plugin && go build -o ./k8s-device-plugin cmd/k8s-device-plugin/main.go

FROM ubuntu:20.04
WORKDIR /root/
COPY --from=gobuild /device-plugin/k8s-device-plugin .
CMD ["./k8s-device-plugin", "-logtostderr=true", "-stderrthreshold=INFO", "-v=5", "--device-config-file=/device-config.yaml"]