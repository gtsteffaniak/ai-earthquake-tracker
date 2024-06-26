FROM golang:1.22-alpine as builder
WORKDIR /app/
COPY [ "./", "./" ]
RUN go build -ldflags='-w -s' .

FROM alpine
WORKDIR /app
COPY --from=builder [ "/app/", "./" ]
CMD ["/app/ai-earthquake-tracker"]
