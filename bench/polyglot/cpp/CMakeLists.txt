# C++ part of the cross-language benchmark (see ../PROTOCOL.md and
# COVERAGE.md). One executable per library (bench-<lib>), linked with the
# harness of the C bench (../c/harness, compiled as C) through its C++ face
# (harness/harness.hpp). Release build with CMake's default flags (-O3
# -DNDEBUG), no CPU-specific option. A library whose dependency is missing is
# skipped at configure time, so that the others still build.
cmake_minimum_required(VERSION 3.20)
project(fx_bench_cpp LANGUAGES C CXX)

if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release CACHE STRING "Build type" FORCE)
endif()
set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

set(FX_ARTEFACTS ${CMAKE_CURRENT_SOURCE_DIR}/.artefacts)

find_package(Threads REQUIRED)
find_package(PkgConfig REQUIRED)

add_library(fx_harness STATIC ../c/harness/harness.c)
target_include_directories(fx_harness PUBLIC ../c/harness harness)
target_compile_definitions(fx_harness PRIVATE
  FX_DEFAULT_ECOSYSTEM="${CMAKE_CURRENT_SOURCE_DIR}/../../ecosystem"
  FX_DEFAULT_ARTEFACTS="${FX_ARTEFACTS}")
target_link_libraries(fx_harness PUBLIC Threads::Threads)

function(fx_bench name)
  add_executable(bench-${name} libs/${name}/main.cpp)
  target_link_libraries(bench-${name} PRIVATE fx_harness ${ARGN})
  message(STATUS "bench-${name}: enabled")
endfunction()

# --- PDF libraries (Homebrew) -------------------------------------------------
find_package(qpdf CONFIG QUIET)
if(TARGET qpdf::libqpdf)
  fx_bench(qpdf qpdf::libqpdf)
else()
  message(WARNING "qpdf not found: bench-qpdf skipped (brew install qpdf)")
endif()

pkg_check_modules(PODOFO IMPORTED_TARGET libpodofo)
if(PODOFO_FOUND)
  fx_bench(podofo PkgConfig::PODOFO)
else()
  message(WARNING "PoDoFo not found: bench-podofo skipped (brew install podofo)")
endif()

pkg_check_modules(POPPLER_CPP IMPORTED_TARGET poppler-cpp)
if(POPPLER_CPP_FOUND)
  fx_bench(poppler-cpp PkgConfig::POPPLER_CPP)
else()
  message(WARNING "poppler-cpp not found: bench-poppler-cpp skipped (brew install poppler)")
endif()

# --- XML libraries (Homebrew) -------------------------------------------------
pkg_check_modules(XERCES IMPORTED_TARGET xerces-c)
if(XERCES_FOUND)
  fx_bench(xerces-c PkgConfig::XERCES)
else()
  message(WARNING "Xerces-C not found: bench-xerces-c skipped (brew install xerces-c)")
endif()

pkg_check_modules(PUGIXML IMPORTED_TARGET pugixml)
if(PUGIXML_FOUND)
  fx_bench(pugixml PkgConfig::PUGIXML)
else()
  message(WARNING "pugixml not found: bench-pugixml skipped (brew install pugixml)")
endif()

# --- SaxonC-HE (binary release unpacked by run.sh setup) ----------------------
# Imported by hand rather than through the CMake package files of the archive,
# so that configuring runs none of its scripts.
set(SAXONC_DIR ${FX_ARTEFACTS}/saxonc/SaxonCHE-macos-arm64-13-1-1)
if(EXISTS ${SAXONC_DIR}/SaxonCHE/lib/libsaxonc-he.13.1.1.dylib AND PUGIXML_FOUND)
  add_library(saxonc_core SHARED IMPORTED)
  set_target_properties(saxonc_core PROPERTIES
    IMPORTED_LOCATION ${SAXONC_DIR}/SaxonCCoreHE/lib/libsaxonc-core-he.13.1.1.dylib
    IMPORTED_SONAME @rpath/libsaxonc-core-he.13.dylib
    INTERFACE_INCLUDE_DIRECTORIES ${SAXONC_DIR}/SaxonCCoreHE/include)
  add_library(saxonc_he SHARED IMPORTED)
  set_target_properties(saxonc_he PROPERTIES
    IMPORTED_LOCATION ${SAXONC_DIR}/SaxonCHE/lib/libsaxonc-he.13.1.1.dylib
    IMPORTED_SONAME @rpath/libsaxonc-he.13.dylib
    INTERFACE_INCLUDE_DIRECTORIES ${SAXONC_DIR}/SaxonCHE/include
    INTERFACE_LINK_LIBRARIES saxonc_core)
  fx_bench(saxonc-he saxonc_he PkgConfig::PUGIXML)
  set_target_properties(bench-saxonc-he PROPERTIES
    BUILD_RPATH "${SAXONC_DIR}/SaxonCHE/lib;${SAXONC_DIR}/SaxonCCoreHE/lib")
else()
  message(WARNING "SaxonC-HE not unpacked (./run.sh setup, macOS arm64 only) or pugixml missing: "
                  "bench-saxonc-he skipped")
endif()

# --- Kura (pinned source unpacked by run.sh setup) ----------------------------
# Its own CMake project, built statically without the optional parts (PDFium
# raster flattening, zxing-cpp barcodes, QuickJS scripts, CLI, fuzzers). It
# needs qpdf, freetype, openjpeg, jpeg-turbo, little-cms2 and zlib.
set(KURA_DIR ${FX_ARTEFACTS}/kura/bentopdf-kura-1.4.1/pdfa-engine)
# Checked first: Kura's own CMakeLists stops the whole configuration when one
# is missing.
find_package(Freetype QUIET)
find_package(JPEG QUIET)
find_package(ZLIB QUIET)
find_package(OpenJPEG CONFIG QUIET)
find_library(FX_LCMS2 NAMES lcms2 PATHS /opt/homebrew/lib /usr/local/lib)
if(EXISTS ${KURA_DIR}/CMakeLists.txt AND TARGET qpdf::libqpdf AND Freetype_FOUND AND JPEG_FOUND
   AND ZLIB_FOUND AND OpenJPEG_FOUND AND FX_LCMS2)
  set(PDFA_BUILD_CLI OFF CACHE BOOL "" FORCE)
  set(PDFA_BUILD_SDK ON CACHE BOOL "" FORCE)
  set(PDFA_BUILD_FUZZ OFF CACHE BOOL "" FORCE)
  set(PDFA_BUILD_SDK_SMOKE OFF CACHE BOOL "" FORCE)
  set(PDFA_WITH_PDFIUM OFF CACHE STRING "" FORCE)
  set(PDFA_STATIC_DEPS OFF CACHE BOOL "" FORCE)
  set(KURA_WITH_ZXING OFF CACHE BOOL "" FORCE)
  set(KURA_WITH_QUICKJS OFF CACHE BOOL "" FORCE)
  add_subdirectory(${KURA_DIR} ${CMAKE_BINARY_DIR}/kura EXCLUDE_FROM_ALL)
  fx_bench(kura kura_sdk pdfa_core)
else()
  message(WARNING "Kura source not unpacked (./run.sh setup) or one of qpdf, freetype, jpeg-turbo, "
                  "openjpeg, little-cms2, zlib missing: bench-kura skipped")
endif()
