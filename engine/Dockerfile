# syntax=docker/dockerfile:1

FROM ubuntu:24.04 AS vcpkg-deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl zip unzip tar pkg-config ninja-build cmake g++ build-essential ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN git clone https://github.com/microsoft/vcpkg.git /opt/vcpkg \
    && /opt/vcpkg/bootstrap-vcpkg.sh
ENV VCPKG_ROOT=/opt/vcpkg
WORKDIR /src/engine

COPY engine/vcpkg.json ./vcpkg.json
RUN /opt/vcpkg/vcpkg install --triplet x64-linux


FROM vcpkg-deps AS build
COPY engine/ /src/engine/
RUN cmake --preset linux-release -S /src/engine \
    && cmake --build /src/engine/out/build/linux-release --target engine -j"$(nproc)"


FROM ubuntu:24.04 AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /src/engine/out/build/linux-release/engine /usr/local/bin/engine
ENV ENGINE_GRPC_PORT=50051
EXPOSE 50051
ENTRYPOINT ["/usr/local/bin/engine"]
