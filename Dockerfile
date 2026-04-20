FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS builder
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build -ldflags="-s -w" -o server .

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8080
CMD ["./server"]
