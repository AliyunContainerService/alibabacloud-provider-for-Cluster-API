# syntax=docker/dockerfile:1.4

############# Builder Stage #############
FROM golang:1.23 AS builder
# Install unzip to extract Terraform and provider zips
RUN apt-get update && apt-get install -y unzip && rm -rf /var/lib/apt/lists/*
ARG TARGETOS
ARG TARGETARCH
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn

WORKDIR /workspace

# Cache Go modules separately for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build manager
COPY . .
RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH:-amd64} \
    go build -a -o bin/manager cmd/main.go

# Download Terraform CLI
ARG TERRAFORM_VERSION=1.2.1
RUN wget -q https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip -d bin/ \
    && rm terraform_${TERRAFORM_VERSION}_linux_amd64.zip

# Download AliCloud Terraform Provider plugin
ARG ALICLOUD_PROVIDER_VERSION=1.223.2
RUN wget -qO terraform-provider-alicloud_v${ALICLOUD_PROVIDER_VERSION}_linux_amd64.zip -L \
    https://github.com/aliyun/terraform-provider-alicloud/releases/download/v${ALICLOUD_PROVIDER_VERSION}/terraform-provider-alicloud_${ALICLOUD_PROVIDER_VERSION}_linux_amd64.zip && \
    mkdir -p .terraform/providers/registry.terraform.io/aliyun/alicloud/${ALICLOUD_PROVIDER_VERSION}/linux_amd64/ && \
    unzip -q terraform-provider-alicloud_v${ALICLOUD_PROVIDER_VERSION}_linux_amd64.zip -d .terraform/providers/registry.terraform.io/aliyun/alicloud/${ALICLOUD_PROVIDER_VERSION}/linux_amd64/ && \
    rm terraform-provider-alicloud_v${ALICLOUD_PROVIDER_VERSION}_linux_amd64.zip

############# Final Stage #############
FROM alpine:3.18

# Install only necessary packages; no upgrade to avoid permission issues
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /

# Copy built binaries
COPY --from=builder /workspace/bin/manager .
COPY --from=builder /workspace/bin/terraform /usr/local/bin/terraform

# Copy provider plugins
COPY --from=builder /workspace/.terraform/providers /root/.terraform/providers

# Inline .terraformrc to configure plugin installation only
RUN mkdir -p /root/.terraform && \
    { \
      echo 'provider_installation {'; \
      echo '  filesystem_mirror {'; \
      echo '    path = "/root/.terraform/providers"'; \
      echo '  }'; \
      echo '  direct {}'; \
      echo '}'; \
      echo 'plugin_cache_dir = "/root/.terraform/plugin-cache"'; \
    } > /root/.terraformrc

ENTRYPOINT ["/manager"]
