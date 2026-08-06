add_executable(kraken_recorder recorder.cpp)
target_link_libraries(kraken_recorder PRIVATE engine_lib)

add_executable(kraken_replayer replayer.cpp)
target_link_libraries(kraken_replayer PRIVATE engine_lib)

add_test(
    NAME replay_golden
    COMMAND kraken_replayer verify
        --in "${CMAKE_CURRENT_SOURCE_DIR}/../../tests/fixtures/golden_session.ptrec"
        --golden "${CMAKE_CURRENT_SOURCE_DIR}/../../tests/fixtures/golden_checksums.txt"
)
