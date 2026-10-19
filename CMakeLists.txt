cmake_minimum_required(VERSION 3.16)

# Milestone 0: ABI/header and packaging contract for the planned native core.
# The Go CLI in tools/hrt-cold is still the working implementation.
project(hrtcold_native
    VERSION 0.1.0
    DESCRIPTION "Experimental native core for the Oyama HRT Tracker cold-backup Tools"
    LANGUAGES C CXX
)

include(GNUInstallDirs)
include(CMakePackageConfigHelpers)

option(HRTCOLD_BUILD_HEADER_CHECKS
    "Build the C and C++ header compatibility checks" ON)
option(HRTCOLD_ENABLE_SANITIZERS
    "Enable AddressSanitizer and UndefinedBehaviorSanitizer (GCC/Clang)" OFF)
option(HRTCOLD_ENABLE_WERROR
    "Treat compiler warnings as errors" OFF)

set(HRTCOLD_CRYPTO_BACKEND "rnp" CACHE STRING
    "Planned native OpenPGP backend: rnp, openssl, botan, libgcrypt, none")
set_property(CACHE HRTCOLD_CRYPTO_BACKEND PROPERTY STRINGS
    rnp openssl botan libgcrypt none)

if(NOT HRTCOLD_CRYPTO_BACKEND MATCHES "^(rnp|openssl|botan|libgcrypt|none)$")
    message(FATAL_ERROR
        "HRTCOLD_CRYPTO_BACKEND must be rnp, openssl, botan, libgcrypt, or none")
endif()

function(hrtcold_enable_warnings target)
    if(MSVC)
        target_compile_options("${target}" PRIVATE /W4)
        if(HRTCOLD_ENABLE_WERROR)
            target_compile_options("${target}" PRIVATE /WX)
        endif()
    else()
        target_compile_options("${target}" PRIVATE -Wall -Wextra -Wpedantic)
        if(HRTCOLD_ENABLE_WERROR)
            target_compile_options("${target}" PRIVATE -Werror)
        endif()
    endif()
endfunction()

if(HRTCOLD_ENABLE_SANITIZERS)
    if(NOT CMAKE_C_COMPILER_ID MATCHES "GNU|Clang")
        message(FATAL_ERROR
            "HRTCOLD_ENABLE_SANITIZERS requires GCC or Clang, got ${CMAKE_C_COMPILER_ID}")
    endif()
    add_compile_options(-fsanitize=address,undefined -fno-omit-frame-pointer)
    add_link_options(-fsanitize=address,undefined)
endif()

# Milestone 0 exposes an interface-only target: it installs the ABI header but
# exports no symbols. When the implementation lands this becomes STATIC/SHARED
# with sources added to the same target name.
add_library(hrtcold_native INTERFACE)
add_library(hrtcold::native ALIAS hrtcold_native)
set_target_properties(hrtcold_native PROPERTIES EXPORT_NAME native)
target_include_directories(hrtcold_native INTERFACE
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:${CMAKE_INSTALL_INCLUDEDIR}>
)

if(HRTCOLD_BUILD_HEADER_CHECKS)
    enable_testing()

    add_executable(hrtcold_header_c tests/native/header_c.c)
    target_link_libraries(hrtcold_header_c PRIVATE hrtcold_native)
    set_target_properties(hrtcold_header_c PROPERTIES
        C_STANDARD 11
        C_STANDARD_REQUIRED ON
        C_EXTENSIONS OFF
    )
    hrtcold_enable_warnings(hrtcold_header_c)

    add_executable(hrtcold_header_cpp tests/native/header_cpp.cpp)
    target_link_libraries(hrtcold_header_cpp PRIVATE hrtcold_native)
    set_target_properties(hrtcold_header_cpp PROPERTIES
        CXX_STANDARD 17
        CXX_STANDARD_REQUIRED ON
        CXX_EXTENSIONS OFF
    )
    hrtcold_enable_warnings(hrtcold_header_cpp)

    add_test(NAME native.header.c COMMAND hrtcold_header_c)
    add_test(NAME native.header.cpp COMMAND hrtcold_header_cpp)
endif()

install(TARGETS hrtcold_native EXPORT hrtcoldTargets)
install(DIRECTORY include/ DESTINATION "${CMAKE_INSTALL_INCLUDEDIR}")
install(EXPORT hrtcoldTargets
    NAMESPACE hrtcold::
    DESTINATION "${CMAKE_INSTALL_LIBDIR}/cmake/hrtcold"
)

configure_package_config_file(
    cmake/hrtcoldConfig.cmake.in
    "${CMAKE_CURRENT_BINARY_DIR}/hrtcoldConfig.cmake"
    INSTALL_DESTINATION "${CMAKE_INSTALL_LIBDIR}/cmake/hrtcold"
)
write_basic_package_version_file(
    "${CMAKE_CURRENT_BINARY_DIR}/hrtcoldConfigVersion.cmake"
    VERSION "${PROJECT_VERSION}"
    COMPATIBILITY SameMajorVersion
)
install(FILES
    "${CMAKE_CURRENT_BINARY_DIR}/hrtcoldConfig.cmake"
    "${CMAKE_CURRENT_BINARY_DIR}/hrtcoldConfigVersion.cmake"
    DESTINATION "${CMAKE_INSTALL_LIBDIR}/cmake/hrtcold"
)

configure_file(cmake/hrtcold.pc.in "${CMAKE_CURRENT_BINARY_DIR}/hrtcold.pc" @ONLY)
install(FILES "${CMAKE_CURRENT_BINARY_DIR}/hrtcold.pc"
    DESTINATION "${CMAKE_INSTALL_LIBDIR}/pkgconfig"
)

message(STATUS "hrtcold native milestone: 0 (header/package contract only)")
message(STATUS "hrtcold planned crypto backend: ${HRTCOLD_CRYPTO_BACKEND}")
message(STATUS "hrtcold header checks: ${HRTCOLD_BUILD_HEADER_CHECKS}")
message(STATUS "hrtcold sanitizers: ${HRTCOLD_ENABLE_SANITIZERS}")
