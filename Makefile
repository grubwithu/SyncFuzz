CLANG ?= clang
BPFTOOL ?= /usr/sbin/bpftool
PYTHON ?= python3
BUILD_DIR ?= build/m1
BPF_ARCH_INCLUDE ?= /usr/include/x86_64-linux-gnu

BPF_SOURCE := syncfuzz/m1/bpf/ktrace.bpf.c
BPF_HEADER := syncfuzz/m1/bpf/ktrace.bpf.h
VMLINUX_HEADER := $(BUILD_DIR)/vmlinux.h
BPF_OBJECT := $(BUILD_DIR)/ktrace.bpf.o
BPF_SKELETON := $(BUILD_DIR)/ktrace.skel.h
M1_CLI := $(BUILD_DIR)/syncfuzz-ktrace
CJSON_DIR := syncfuzz/third_party/cjson
CJSON_SOURCE := $(CJSON_DIR)/cJSON.c
CJSON_HEADER := $(CJSON_DIR)/cJSON.h
M1_MANIFEST_SOURCE := syncfuzz/m1/manifest.c
M1_MANIFEST_HEADER := syncfuzz/m1/manifest.h

.PHONY: m1-build m1-test m1-run m1-acceptance

m1-build: $(M1_CLI)

m1-clean:
	rm -rf "$(BUILD_DIR)"

m1-test:
	$(PYTHON) -m pytest tests/m1

m1-acceptance: $(M1_CLI)
	$(PYTHON) tests/m1/ktrace_acceptance_runner.py --collector "$(M1_CLI)"

m1-run: $(M1_CLI)
	@test -n "$(CGROUP_ID)" || (echo "CGROUP_ID is required" >&2; exit 2)
	@test -n "$(OUT)" || (echo "OUT is required" >&2; exit 2)
	@test -n "$(MARKER_SO)" || (echo "MARKER_SO is required" >&2; exit 2)
	@test -n "$(MANIFEST)" || (echo "MANIFEST is required" >&2; exit 2)
	mkdir -p "$(dir $(OUT))"
	$(M1_CLI) --cgroup-id "$(CGROUP_ID)" --out "$(OUT)" --duration "$(or $(DURATION),10)" \
		--marker-so "$(MARKER_SO)" --manifest "$(MANIFEST)"

$(VMLINUX_HEADER):
	mkdir -p "$(BUILD_DIR)"
	$(BPFTOOL) btf dump file /sys/kernel/btf/vmlinux format c > "$@"

$(BPF_OBJECT): $(BPF_SOURCE) $(BPF_HEADER) $(VMLINUX_HEADER)
	$(CLANG) -g -O2 -target bpf -D__TARGET_ARCH_x86 -I "$(BUILD_DIR)" \
		-I "$(BPF_ARCH_INCLUDE)" -c "$(BPF_SOURCE)" -o "$@"

$(BPF_SKELETON): $(BPF_OBJECT)
	$(BPFTOOL) gen skeleton "$<" > "$@"

$(M1_CLI): syncfuzz/m1/ktrace.c $(M1_MANIFEST_SOURCE) $(M1_MANIFEST_HEADER) $(CJSON_SOURCE) $(CJSON_HEADER) $(BPF_SKELETON)
	$(CLANG) -std=c11 -Wall -Wextra -Werror -I "$(BUILD_DIR)" -I "$(CJSON_DIR)" \
		syncfuzz/m1/ktrace.c "$(M1_MANIFEST_SOURCE)" "$(CJSON_SOURCE)" -lbpf -lz -o "$@"
