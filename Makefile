include third_party_versions.mk

EMSDK_DIR=$(PWD)/third_party/emsdk/upstream/emscripten
INSTALL_DIR=$(PWD)/install
FALLBACK_INSTALL_DIR=$(INSTALL_DIR)/fallback

DIST_TARGETS=\
						 build/tesseract-core.wasm

.PHONY: lib
lib: $(DIST_TARGETS)

clean:
	rm -rf build dist install

clean-lib:
	rm build/*.{wasm}
	rm -rf dist

# nb. This is an order-only dependency in other targets.
build:
	mkdir -p build/

third_party/emsdk: third_party_versions.mk
	mkdir -p third_party/emsdk
	test -d $@/.git || git clone --depth 1 https://github.com/emscripten-core/emsdk.git $@
	cd $@ && git fetch origin $(EMSDK_COMMIT) && git checkout $(EMSDK_COMMIT)
	touch $@

build/emsdk.uptodate: third_party/emsdk
	third_party/emsdk/emsdk install 3.1.37
	third_party/emsdk/emsdk activate 3.1.37
	rm -Rf third_party/emsdk/upstream/emscripten
	git clone $(EMSCRIPTEN_FORK) third_party/emsdk/upstream/emscripten
	cd third_party/emsdk/upstream/emscripten/ && git checkout $(EMSCRIPTEN_BRANCH)
	$(EMSDK_DIR)/embuilder build zlib
	$(EMSDK_DIR)/embuilder build libpng
	$(EMSDK_DIR)/embuilder build libjpeg
	touch build/emsdk.uptodate

LEPTONICA_DEP_FLAGS=-s USE_ZLIB=1 -s USE_LIBPNG=1 -s USE_LIBJPEG=1
LEPTONICA_FLAGS=-DCMAKE_INSTALL_PREFIX="$(INSTALL_DIR)" \
								-DCMAKE_TOOLCHAIN_FILE="$(EMSDK_DIR)/cmake/Modules/Platform/Emscripten.cmake" \
								-DCMAKE_CXX_FLAGS="$(LEPTONICA_DEP_FLAGS)" \
								-DCMAKE_C_FLAGS="$(LEPTONICA_DEP_FLAGS)" \
								-DCMAKE_EXE_LINKER_FLAGS=" $(LEPTONICA_DEP_FLAGS)"

third_party/leptonica: third_party_versions.mk
	mkdir -p third_party/leptonica
	test -d $@/.git || git clone --depth 1 https://github.com/DanBloomberg/leptonica.git $@
	cd $@ && git fetch origin $(LEPTONICA_COMMIT) && git checkout $(LEPTONICA_COMMIT)
	touch $@

build/leptonica.uptodate: third_party/leptonica build/emsdk.uptodate
	mkdir -p build/leptonica
	cd build/leptonica && $(EMSDK_DIR)/emcmake cmake -G Ninja ../../third_party/leptonica $(LEPTONICA_FLAGS)
	cd build/leptonica && $(EMSDK_DIR)/emmake ninja
	cd build/leptonica && $(EMSDK_DIR)/emmake ninja install
	touch build/leptonica.uptodate

# Additional preprocessor defines for Tesseract.
#
# Defining `TESSERACT_IMAGEDATA_AS_PIX` disables some unnecessary internal use
# of the PNG format. See Tesseract commit 6bcb941bcff5e73b62ecc8d2aa5691d3e0e7afc0.
TESSERACT_DEFINES=

# Compile flags for Tesseract. These turn off support for unused features and
# utility programs to reduce size and build times.
#
# 128-bit wide SIMD is enabled via `HAVE_SSE4_1` and the `-msimd128` flags. The
# AVX flags are disabled because they require instructions beyond what WASM SIMD
# supports.
TESSERACT_FLAGS=\
								-DBUILD_TESSERACT_BINARY=OFF \
								-DBUILD_TRAINING_TOOLS=OFF \
								-DDISABLE_CURL=ON \
								-DDISABLED_LEGACY_ENGINE=ON \
								-DENABLE_LTO=ON \
								-DGRAPHICS_DISABLED=ON \
								-DHAVE_AVX=OFF \
								-DHAVE_AVX2=OFF \
								-DHAVE_AVX512F=OFF \
								-DHAVE_FMA=OFF \
								-DHAVE_SSE4_1=ON \
								-DLeptonica_DIR=$(INSTALL_DIR)/lib/cmake/leptonica \
								-DCMAKE_CXX_FLAGS="$(TESSERACT_DEFINES) -msimd128 -s USE_ZLIB=1 -s USE_LIBPNG=1 -s USE_LIBJPEG=1" \
								-DCMAKE_INSTALL_PREFIX=$(INSTALL_DIR) \
								-DCMAKE_TOOLCHAIN_FILE="$(EMSDK_DIR)/cmake/Modules/Platform/Emscripten.cmake"


third_party/tesseract: third_party_versions.mk
	mkdir -p third_party/tesseract
	test -d $@/.git || git clone --depth 1 https://github.com/tesseract-ocr/tesseract.git $@
	cd $@ && git fetch origin $(TESSERACT_COMMIT) && git checkout $(TESSERACT_COMMIT)
	cd $@ && git stash && git apply ../../patches/tesseract.diff
	touch $@

third_party/tessdata_fast:
	mkdir -p third_party/tessdata_fast
	git clone --depth 1 https://github.com/tesseract-ocr/tessdata_fast.git $@

build/tesseract.uptodate: build/leptonica.uptodate third_party/tesseract
	mkdir -p build/tesseract
	(cd build/tesseract && $(EMSDK_DIR)/emcmake cmake -G Ninja ../../third_party/tesseract $(TESSERACT_FLAGS))
	(cd build/tesseract && $(EMSDK_DIR)/emmake ninja)
	(cd build/tesseract && $(EMSDK_DIR)/emmake ninja install)
	touch build/tesseract.uptodate


EXPORTED_FUNCTIONS=$(shell (cat tessbridge/tessbridge.h | sed -nr 's/.* \*?([A-Z][a-zA-Z0-9]*)\(.*\);/\1/p' | sed 's/^/_/' | paste -sd "," -))
EMCC_FLAGS =\
						-Oz\
						-sEXPORTED_FUNCTIONS="_malloc,_free,$(EXPORTED_FUNCTIONS)"\
						-sSTANDALONE_WASM\
						-sWARN_ON_UNDEFINED_SYMBOLS=0\
						--no-entry\
						-sFILESYSTEM=1\
						-sALLOW_MEMORY_GROWTH\
						-sMAXIMUM_MEMORY=1GB\
						-std=c++20\
						-s USE_ZLIB=1 -s USE_LIBPNG=1 -s USE_LIBJPEG=1\
						-g

build/tesseract-core.wasm: tessbridge/tessbridge.cpp build/tesseract.uptodate
	$(EMSDK_DIR)/emcc --version
	$(EMSDK_DIR)/emcc tessbridge/tessbridge.cpp $(EMCC_FLAGS) \
		-I$(INSTALL_DIR)/include/ -L$(INSTALL_DIR)/lib/ -ltesseract -lleptonica\
		-o build/tesseract-core.wasm

