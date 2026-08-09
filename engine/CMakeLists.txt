cmake_minimum_required(VERSION 3.20)
project(paper_trader_engine CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

find_package(Boost REQUIRED)
find_package(OpenSSL REQUIRED)
find_package(ZLIB REQUIRED)
find_package(simdjson CONFIG REQUIRED)
find_package(Protobuf CONFIG REQUIRED)
find_package(gRPC CONFIG REQUIRED)

set(PROTO_FILE "${CMAKE_CURRENT_SOURCE_DIR}/proto/paper_trader.proto")
set(PROTO_GEN_DIR "${CMAKE_CURRENT_BINARY_DIR}/generated")
file(MAKE_DIRECTORY "${PROTO_GEN_DIR}")

set(PROTO_SRCS "${PROTO_GEN_DIR}/paper_trader.pb.cc")
set(PROTO_HDRS "${PROTO_GEN_DIR}/paper_trader.pb.h")
set(GRPC_SRCS "${PROTO_GEN_DIR}/paper_trader.grpc.pb.cc")
set(GRPC_HDRS "${PROTO_GEN_DIR}/paper_trader.grpc.pb.h")

add_custom_command(
    OUTPUT ${PROTO_SRCS} ${PROTO_HDRS} ${GRPC_SRCS} ${GRPC_HDRS}
    COMMAND protobuf::protoc
    ARGS --cpp_out=${PROTO_GEN_DIR}
         --grpc_out=${PROTO_GEN_DIR}
         --plugin=protoc-gen-grpc=$<TARGET_FILE:gRPC::grpc_cpp_plugin>
         -I "${CMAKE_CURRENT_SOURCE_DIR}/proto"
         "${PROTO_FILE}"
    DEPENDS "${PROTO_FILE}"
    COMMENT "Generating C++ bindings from paper_trader.proto"
)

add_library(engine_lib STATIC
    src/book/order_book.cpp
    src/book/checksum.cpp
    src/matching/matching_engine.cpp
    src/market_data/kraken_asset_pairs.cpp
    src/market_data/kraken_book_client.cpp
    src/market_data/kraken_parser.cpp
    src/ipc/shared_memory_manager.cpp
    src/control/control_server.cpp
    src/symbol/symbol_registry.cpp
    src/observability/histogram.cpp
    ${PROTO_SRCS}
    ${GRPC_SRCS}
)

target_include_directories(engine_lib PUBLIC
    include
    "${PROTO_GEN_DIR}"
)

if(NOT DEFINED SHM_DIR)
    get_filename_component(SHM_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../.shm" ABSOLUTE)
endif()
target_compile_definitions(engine_lib PUBLIC
    BOOST_INTERPROCESS_SHARED_DIR_PATH="${SHM_DIR}"
)

target_link_libraries(engine_lib PUBLIC
    Boost::boost
    OpenSSL::SSL
    OpenSSL::Crypto
    ZLIB::ZLIB
    simdjson::simdjson
    gRPC::grpc++
    protobuf::libprotobuf
)

add_executable(engine src/main.cpp)
target_link_libraries(engine PRIVATE engine_lib)

enable_testing()
add_subdirectory(tests)
add_subdirectory(tools/replay)
