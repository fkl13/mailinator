# Build application
FROM golang:1.27 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 go build -o mailinator .

# Build lean image
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=builder /app/mailinator /mailinator
EXPOSE 8080
EXPOSE 2525
ENTRYPOINT ["/mailinator"]
