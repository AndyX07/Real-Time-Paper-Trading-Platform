# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
RUN apk add --no-cache protobuf bash
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY engine/proto/ /src/engine/proto/

RUN bash scripts/gen_proto.sh
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12 AS runtime
COPY --from=build /out/server /server
ENV HOST=0.0.0.0
ENV PORT=8000
EXPOSE 8000
ENTRYPOINT ["/server"]
