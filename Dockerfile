##Builder Image
FROM golang:1.17-stretch as builder
ENV GO111MODULE=on
COPY . /matches
WORKDIR /matches
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/application

#s Run Image
FROM scratch
COPY --from=builder /matches/assets /assets
COPY --from=builder /matches/bin/application application
EXPOSE 9999
ENTRYPOINT ["./application"]