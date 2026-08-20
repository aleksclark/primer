FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/external-verifier-fixture .

FROM alpine:3.22
RUN addgroup -S verifier && adduser -S -G verifier verifier && mkdir -p /var/lib/external-verifier && chown verifier:verifier /var/lib/external-verifier
COPY --from=build /out/external-verifier-fixture /usr/local/bin/external-verifier-fixture
USER verifier
EXPOSE 8092
ENTRYPOINT ["/usr/local/bin/external-verifier-fixture"]
