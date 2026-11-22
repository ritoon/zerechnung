# C part of the cross-language benchmark (see ../PROTOCOL.md and COVERAGE.md).
# One executable per library (bench-<lib>), linked with the shared harness
# (harness/). Release build with CMake's default flags (-O3 -DNDEBUG), no
# CPU-specific option. A library whose dependency is missing is skipped at
# configure time, so that the others still build.
cmake_minimum_required(VERSION 3.20)
project(fx_bench_c LANGUAGES C)

if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release CACHE STRING "Build type" FORCE)
endif()
set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)

find_package(Threads REQUIRED)
find_package(PkgConfig REQUIRED)

add_library(fx_harness STATIC harness/harness.c)
target_include_directories(fx_harness PUBLIC harness)
target_compile_definitions(fx_harness PRIVATE
  FX_DEFAULT_ECOSYSTEM="${CMAKE_CURRENT_SOURCE_DIR}/../../ecosystem"
  FX_DEFAULT_ARTEFACTS="${CMAKE_CURRENT_SOURCE_DIR}/.artefacts")
target_link_libraries(fx_harness PUBLIC Threads::Threads)

function(fx_bench name)
  add_executable(bench-${name} libs/${name}/main.c)
  target_link_libraries(bench-${name} PRIVATE fx_harness ${ARGN})
  message(STATUS "bench-${name}: enabled")
endfunction()

# libxml2 and libxslt are keg-only in Homebrew: run.sh passes their prefixes
# in CMAKE_PREFIX_PATH, which pkg-config searches too.
pkg_check_modules(LIBXML2 IMPORTED_TARGET libxml-2.0)
pkg_check_modules(LIBXSLT IMPORTED_TARGET libxslt)
# libqpdf.pc adds -std=c++20 to its Cflags: the C API is found directly.
find_path(QPDF_INCLUDE_DIR qpdf/qpdf-c.h HINTS /opt/homebrew/opt/qpdf/include)
find_library(QPDF_LIBRARY qpdf HINTS /opt/homebrew/opt/qpdf/lib)

find_path(MUPDF_INCLUDE_DIR mupdf/fitz.h HINTS /opt/homebrew/opt/mupdf/include)
find_library(MUPDF_LIBRARY mupdf HINTS /opt/homebrew/opt/mupdf/lib)

if(MUPDF_INCLUDE_DIR AND MUPDF_LIBRARY)
  fx_bench(mupdf ${MUPDF_LIBRARY})
  target_include_directories(bench-mupdf PRIVATE ${MUPDF_INCLUDE_DIR})
else()
  message(WARNING "MuPDF not found: bench-mupdf skipped (brew install mupdf)")
endif()

if(QPDF_INCLUDE_DIR AND QPDF_LIBRARY)
  fx_bench(qpdf-c ${QPDF_LIBRARY})
  target_include_directories(bench-qpdf-c PRIVATE ${QPDF_INCLUDE_DIR})
else()
  message(WARNING "qpdf not found: bench-qpdf-c skipped (brew install qpdf)")
endif()

if(LIBXML2_FOUND)
  fx_bench(libxml2 PkgConfig::LIBXML2)
else()
  message(WARNING "libxml2 not found: bench-libxml2 skipped (brew install libxml2)")
endif()

if(LIBXSLT_FOUND AND LIBXML2_FOUND)
  fx_bench(libxslt PkgConfig::LIBXSLT PkgConfig::LIBXML2)
else()
  message(WARNING "libxslt not found: bench-libxslt skipped (brew install libxslt)")
endif()
