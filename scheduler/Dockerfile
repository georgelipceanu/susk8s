FROM golang:1.24 AS builder
WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o kube-scheduler ./cmd/kube-scheduler

FROM registry.k8s.io/kube-scheduler:v1.30.0
COPY --from=builder /workspace/kube-scheduler /usr/local/bin/kube-scheduler