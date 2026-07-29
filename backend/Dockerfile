FROM golang:1.25-alpine AS build
WORKDIR /app
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /construction-api ./cmd/api

FROM build AS test
RUN go test -timeout 30s ./...
RUN go vet ./...
RUN go mod verify

FROM alpine:3.21
RUN adduser -D -H appuser
COPY --from=build /app/internal/delivery/http/assets/fonts/OFL.txt /licenses/noto-sans-jp/OFL.txt
COPY --from=build /app/internal/delivery/http/assets/fonts/README.md /licenses/noto-sans-jp/README.md
USER appuser
COPY --from=build /construction-api /construction-api
EXPOSE 8080
ENTRYPOINT ["/construction-api"]
