FROM golang:1.26-alpine AS builder

RUN apk --no-cache add git

RUN go install go.k6.io/xk6/cmd/xk6@latest

RUN xk6 build --with github.com/suppachai-n/xk6-gcp@latest \
  --with github.com/szkiba/xk6-yaml@latest \
  --with github.com/szkiba/xk6-csv@latest \
  --with github.com/mostafa/xk6-kafka@latest \
  --with github.com/grafana/xk6-kubernetes@latest \
  --with github.com/grafana/xk6-sql@latest \
  --with github.com/deejiw/xk6-interpret@latest \
  --with github.com/nuttapon-f/xk6-crypto-box@latest \
  --with github.com/ekanant/xk6-crypto-x25519@latest \
  --with github.com/ekanant/xk6-aes-ecb@latest \
  --with github.com/ekanant/xk6-rsa@latest \
  --with github.com/oleiade/xk6-kv@latest && \
  cp k6 $GOPATH/bin/k6

FROM alpine:3.22

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 12345 -g 12345 k6

COPY --from=builder /go/k6 /usr/bin/k6

USER 12345

ENTRYPOINT ["k6"]