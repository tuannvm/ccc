.PHONY: build build-voice install install-voice clean deps test debug deploy

PREFIX := $(CURDIR)/build/whisper
BUILD_DIR := $(CURDIR)/build/cmake
UNAME := $(shell uname)
PC_DIR := $(PREFIX)/lib/pkgconfig

# Default build: no voice/whisper support (works on all platforms)
build:
	go build -o ccc
	@if [ "$(UNAME)" = "Darwin" ]; then \
		codesign -f -s - ccc 2>/dev/null || true; \
	fi

# Build whisper.cpp C library (needed for voice support)
deps:
	@if [ ! -f "$(PREFIX)/lib/libwhisper.a" ]; then \
		echo "Building whisper.cpp..."; \
		git submodule update --init --recursive; \
		cmake -S third_party/whisper.cpp -B $(BUILD_DIR) \
			-DCMAKE_BUILD_TYPE=Release \
			-DBUILD_SHARED_LIBS=OFF \
			-DWHISPER_BUILD_TESTS=OFF \
			-DWHISPER_BUILD_EXAMPLES=OFF \
			-DWHISPER_BUILD_SERVER=OFF; \
		cmake --build $(BUILD_DIR) --config Release -j$$(nproc 2>/dev/null || sysctl -n hw.ncpu); \
		cmake --install $(BUILD_DIR) --prefix $(PREFIX); \
	else \
		echo "whisper.cpp already built"; \
	fi
	@# Generate pkg-config files matching go-whisper expectations
	@mkdir -p "$(PC_DIR)"
	@printf 'prefix=%s\nlibdir=$${prefix}/lib\nincludedir=$${prefix}/include\n\nName: libwhisper\nDescription: whisper.cpp\nVersion: 0.0.0\nCflags: -I$${includedir}\n' "$(PREFIX)" > "$(PC_DIR)/libwhisper.pc"
	@if [ "$(UNAME)" = "Darwin" ]; then \
		printf 'prefix=%s\nlibdir=$${prefix}/lib\n\nName: libwhisper-darwin\nDescription: whisper.cpp (darwin)\nVersion: 0.0.0\nLibs: -L$${libdir} -lwhisper -lggml -lggml-base -lggml-cpu -lggml-blas -lggml-metal -lstdc++ -framework Accelerate -framework Metal -framework Foundation -framework CoreGraphics\n' "$(PREFIX)" > "$(PC_DIR)/libwhisper-darwin.pc"; \
	else \
		printf 'prefix=%s\nlibdir=$${prefix}/lib\n\nName: libwhisper-linux\nDescription: whisper.cpp (linux)\nVersion: 0.0.0\nCflags: -fopenmp\nLibs: -L$${libdir} -lwhisper -lggml -lggml-base -lggml-cpu -lgomp -lm -lstdc++ -lpthread\n' "$(PREFIX)" > "$(PC_DIR)/libwhisper-linux.pc"; \
	fi

# Build with voice/whisper support (requires FFmpeg 7.x + cmake)
build-voice: deps
	PKG_CONFIG_PATH="$(PC_DIR)" CGO_LDFLAGS_ALLOW="-(W|D).*" \
		go build -tags voice -o ccc
	@if [ "$(UNAME)" = "Darwin" ]; then \
		codesign -f -s - ccc 2>/dev/null || true; \
	fi

install: build
	mkdir -p ~/bin
	install -m 755 ccc ~/bin/ccc
	@if [ "$(UNAME)" = "Darwin" ]; then \
		codesign -f -s - ~/bin/ccc 2>/dev/null || true; \
	fi
	@echo "✅ Installed to ~/bin/ccc"

install-voice: build-voice
	mkdir -p ~/bin
	install -m 755 ccc ~/bin/ccc
	@if [ "$(UNAME)" = "Darwin" ]; then \
		codesign -f -s - ~/bin/ccc 2>/dev/null || true; \
	fi
	@echo "✅ Installed to ~/bin/ccc (with voice support)"

clean:
	rm -f ccc
	rm -rf build/

# test: run Go tests
test:
	@echo "🧪 Running tests..."
	go test -v ./...

# debug: build with race detector and debug symbols, then install and restart
debug:
	@echo "🐛 Building debug version with race detector..."
	go build -race -gcflags="all=-N -l" -o ccc
	@if [ "$(UNAME)" = "Darwin" ]; then \
		codesign -f -s - ccc 2>/dev/null || true; \
	fi
	@echo "📦 Installing debug binary to ~/bin/ccc..."
	@install -m 755 ccc ~/bin/ccc
	@echo "🔄 Restarting ccc service..."
	@if [ "$(UNAME)" = "Darwin" ]; then \
		launchctl unload ~/Library/LaunchAgents/com.ccc.plist 2>/dev/null || true; \
		sleep 1; \
		launchctl load ~/Library/LaunchAgents/com.ccc.plist; \
		echo "✅ Debug version deployed (launchd)"; \
	else \
		systemctl --user daemon-reload 2>/dev/null || true; \
		systemctl --user restart ccc.service 2>/dev/null || true; \
		echo "✅ Debug version deployed (systemd)"; \
	fi
	@echo "🐛 Debug mode: race detection enabled, optimizations disabled"

# deploy: build, install, stop active ccc runtime panes, and restart the ccc service
deploy: install
	@echo "🧹 Stopping active ccc tmux runtime..."
	@if command -v tmux >/dev/null 2>&1 && tmux has-session -t ccc 2>/dev/null; then \
		tmux kill-session -t ccc; \
		echo "✅ ccc tmux session stopped"; \
	else \
		echo "ℹ️ No ccc tmux session running"; \
	fi
	@echo "🧹 Stopping stale ccc run processes..."
	@pids=$$(pgrep -f "$$HOME/bin/ccc run" 2>/dev/null || true); \
	if [ -n "$$pids" ]; then \
		echo "$$pids" | xargs kill 2>/dev/null || true; \
		echo "✅ stale ccc run processes stopped"; \
	else \
		echo "ℹ️ No stale ccc run processes found"; \
	fi
	@echo "🔄 Restarting ccc service..."
	@if [ "$(UNAME)" = "Darwin" ]; then \
		launchctl unload ~/Library/LaunchAgents/com.ccc.plist 2>/dev/null || true; \
		sleep 1; \
		launchctl load ~/Library/LaunchAgents/com.ccc.plist; \
		echo "✅ ccc service restarted (launchd)"; \
	else \
		systemctl --user daemon-reload 2>/dev/null || true; \
		systemctl --user restart ccc.service 2>/dev/null || true; \
		echo "✅ ccc service restarted (systemd)"; \
	fi
