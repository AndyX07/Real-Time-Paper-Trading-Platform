find_package(Catch2 CONFIG REQUIRED)

add_executable(engine_tests
    test_order_book.cpp
    test_matching_engine.cpp
    test_checksum_resync.cpp
    test_ring_buffer_stress.cpp
    test_snapshot_slot.cpp
)

target_link_libraries(engine_tests PRIVATE
    engine_lib
    Catch2::Catch2WithMain
)

# Standalone second process for test_ring_buffer_stress.cpp's
# cross-process case -- not a Catch2 test binary itself, just a small
# writer/reader driver the test spawns and inspects via a result file.
add_executable(ring_buffer_stress_helper ring_buffer_stress_helper.cpp)
target_link_libraries(ring_buffer_stress_helper PRIVATE engine_lib)

target_compile_definitions(engine_tests PRIVATE
    RING_BUFFER_STRESS_HELPER_PATH="$<TARGET_FILE:ring_buffer_stress_helper>"
    CHECKSUM_RESYNC_FIXTURE_PATH="${CMAKE_CURRENT_SOURCE_DIR}/fixtures/golden_session.ptrec")
add_dependencies(engine_tests ring_buffer_stress_helper)

include(CTest)
include(Catch)
catch_discover_tests(engine_tests)
