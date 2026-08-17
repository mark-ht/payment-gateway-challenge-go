FROM golang:1.21 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway .

FROM builder AS e2e-builder

RUN CGO_ENABLED=0 GOOS=linux go test -c -tags=e2e -trimpath -ldflags="-s -w" -o /out/e2e ./test/e2e

FROM gcr.io/distroless/static-debian12:nonroot AS gateway

COPY --from=builder --chown=nonroot:nonroot /out/gateway /gateway

USER nonroot:nonroot
EXPOSE 8090
ENTRYPOINT ["/gateway"]

FROM gcr.io/distroless/static-debian12:nonroot AS e2e

COPY --from=e2e-builder --chown=nonroot:nonroot /out/e2e /e2e

USER nonroot:nonroot
ENTRYPOINT ["/e2e"]
