FROM golang:1.27 AS builder
WORKDIR /app
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -o mailinator .

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=builder /app/mailinator /mailinator
EXPOSE 8080
ENTRYPOINT ["/mailinator"]
