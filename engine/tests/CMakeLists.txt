find_package(Catch2 CONFIG REQUIRED)

option(ENABLE_TSAN "Build engine_tests with ThreadSanitizer" OFF)

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

if(ENABLE_TSAN)
    target_compile_options(engine_tests PRIVATE -fsanitize=thread -g)
    target_link_options(engine_tests PRIVATE -fsanitize=thread)
endif()

include(CTest)
include(Catch)
catch_discover_tests(engine_tests)
