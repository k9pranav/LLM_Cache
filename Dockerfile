FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o server ./cmd/server


FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/server .

# Copy config folder to where your app is looking
COPY configs /app/configs

EXPOSE 8080

CMD ["./server"]