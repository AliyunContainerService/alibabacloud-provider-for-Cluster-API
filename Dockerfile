# Build the manager binary
FROM golang:1.21 AS builder
ARG TARGETOS
ARG TARGETARCH

ENV GO111MODULE on
ENV GOPROXY https://goproxy.cn

WORKDIR /workspace
COPY . .

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o bin/manager cmd/main.go
RUN chmod 755 ./bin/*

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine
RUN apk --update upgrade && apk add ca-certificates tzdata && rm -rf /var/cache/apk/*
WORKDIR /
COPY --from=builder /workspace/bin/manager .
COPY .terraformrc /root/.terraformrc
## [download]: https://releases.hashicorp.com/terraform/1.2.1/terraform_1.2.1_linux_amd64.zip
COPY --from=builder /workspace/bin/terraform /usr/local/bin/terraform
## [download]: https://github.com/aliyun/terraform-provider-alicloud/releases/tag/v1.223.2
COPY --from=builder /workspace/bin/terraform-provider-alicloud_v1.223.2 /root/.terraform/providers/registry.terraform.io/aliyun/alicloud/1.223.2/linux_amd64/terraform-provider-alicloud_v1.223.2

ENTRYPOINT ["/manager"]
