add_executable(kraken_recorder recorder.cpp)
target_link_libraries(kraken_recorder PRIVATE engine_lib)

add_executable(kraken_replayer replayer.cpp)
target_link_libraries(kraken_replayer PRIVATE engine_lib)

# Test-only: backs backend/internal/integration/engine_cross_process_test.go.
# Never installed/shipped; see test_engine_harness.cpp for why it exists.
add_executable(test_engine_harness test_engine_harness.cpp)
target_link_libraries(test_engine_harness PRIVATE engine_lib)
target_compile_definitions(test_engine_harness PRIVATE
    TEST_ENGINE_HARNESS_FIXTURE_PATH="${CMAKE_CURRENT_SOURCE_DIR}/../../tests/fixtures/golden_session.ptrec")

add_test(
    NAME replay_golden
    COMMAND kraken_replayer verify
        --in "${CMAKE_CURRENT_SOURCE_DIR}/../../tests/fixtures/golden_session.ptrec"
        --golden "${CMAKE_CURRENT_SOURCE_DIR}/../../tests/fixtures/golden_checksums.txt"
)
