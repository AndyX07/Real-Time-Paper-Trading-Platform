add_executable(kraken_recorder recorder.cpp)
target_link_libraries(kraken_recorder PRIVATE engine_lib)

add_executable(kraken_replayer replayer.cpp)
target_link_libraries(kraken_replayer PRIVATE engine_lib)
