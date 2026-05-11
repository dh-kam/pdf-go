.PHONY: all build build-pdf-go build-cli build-examples build-pdfinfo build-pdftext build-pdfrender build-all build-pure build-all-cgo build-no-cgo test coverage lint mock-gen clean run-tests test-no-cgo test-no-cgo-race coverage-no-cgo coverage-core-no-cgo render-regression-no-cgo check-no-cgo vet-no-cgo cli-smoke-no-cgo vuln-no-cgo goal98-batch-test-no-cgo goal98-batch-no-cgo goal98-html-no-cgo sample-compare-html-no-cgo sample-compare-tradeoff-no-cgo sample-compare-backlog-no-cgo sample-compare-focus-no-cgo sample-compare-faildocs-recheck-no-cgo sample-compare-iccbased-focus-no-cgo nightly-snapshot-no-cgo nightly-compare-diff-no-cgo profile-render-no-cgo profile-render-guard-no-cgo render-leak-check-no-cgo goal98-compare-report goal98-batch-compare-no-cgo goal98-guard-no-cgo pdfjs-select-eq pdfjs-render-parity pdfjs-parity-clean render-parity-report-test lex-render-parity-report-test render-parity-priority porting-complete porting-complete-plus-goal98 release-preflight release-dry-run release-publish

# Build variables
BINARY_NAME=pdfrender
MAIN_BINARY_NAME=pdf-go
BUILD_DIR=bin
BUILD_ROOT=build
CMD_DIR=./cmd
EXAMPLES_DIR=./examples

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint
HOST_OS=$(shell $(GOCMD) env GOOS)
HOST_ARCH=$(shell $(GOCMD) env GOARCH)
OS_LIST?=linux darwin windows
ARCH_LIST?=amd64 arm64
BUILD_VARIANTS?=default nocgo release
TARGET_OS?=$(HOST_OS)
TARGET_ARCH?=$(HOST_ARCH)
TARGET_VARIANT?=default
comma:=,
NO_CGO_TAGS=nojpx$(comma)nojbig2$(comma)nofreetype$(comma)nocairo
TARGET_CGO_ENABLED?=$(if $(filter default,$(TARGET_VARIANT)),1,0)
TARGET_TAGS?=$(if $(filter nocgo release,$(TARGET_VARIANT)),$(NO_CGO_TAGS),)
GO_DEFAULT_FLAGS?=-trimpath
GO_RELEASE_FLAGS?=-trimpath -ldflags="-s -w"
TARGET_GOFLAGS?=$(if $(filter release,$(TARGET_VARIANT)),$(GO_RELEASE_FLAGS),$(GO_DEFAULT_FLAGS))
TARGET_TAG_FLAGS=$(if $(TARGET_TAGS),-tags='$(TARGET_TAGS)',)
TARGET_EXE=$(if $(filter windows,$(TARGET_OS)),.exe,)
HOST_EXE=$(if $(filter windows,$(HOST_OS)),.exe,)
TARGET_BUILD_DIR=$(BUILD_ROOT)/$(TARGET_OS)-$(TARGET_ARCH)/$(TARGET_VARIANT)
CORE_CLI_TOOLS=pdfinfo pdftext pdfrender
CLI_PACKAGE_DIRS=$(sort $(dir $(wildcard $(CMD_DIR)/*/main.go)))
CLI_FILE_SOURCES=$(filter-out %_test.go,$(wildcard $(CMD_DIR)/*.go))
EXAMPLE_SOURCES=$(filter-out %_test.go,$(wildcard $(EXAMPLES_DIR)/*.go))
rwildcard=$(wildcard $(1)/$(2)) $(foreach d,$(wildcard $(1)/*),$(call rwildcard,$(d),$(2)))
GO_FILES=$(call rwildcard,.,*.go)
build_stamp=$(BUILD_ROOT)/$(1)-$(2)/$(3)/.complete
FULL_BUILD_TARGETS=$(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(call build_stamp,$(os),$(arch),$(var)))))
OS_ARCH_PAIRS=$(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(os)-$(arch)))
OS_VARIANT_PAIRS=$(foreach os,$(OS_LIST),$(foreach var,$(BUILD_VARIANTS),$(os)-$(var)))
ARCH_VARIANT_PAIRS=$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(arch)-$(var)))
FULL_SELECTOR_TARGETS=$(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(os)-$(arch)-$(var))))
token1=$(word 1,$(subst -, ,$(1)))
token2=$(word 2,$(subst -, ,$(1)))
token3=$(word 3,$(subst -, ,$(1)))
all_for_os=$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(call build_stamp,$(1),$(arch),$(var))))
all_for_arch=$(foreach os,$(OS_LIST),$(foreach var,$(BUILD_VARIANTS),$(call build_stamp,$(os),$(1),$(var))))
all_for_variant=$(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(call build_stamp,$(os),$(arch),$(1))))
all_for_os_arch=$(foreach var,$(BUILD_VARIANTS),$(call build_stamp,$(1),$(2),$(var)))
all_for_os_variant=$(foreach arch,$(ARCH_LIST),$(call build_stamp,$(1),$(arch),$(2)))
all_for_arch_variant=$(foreach os,$(OS_LIST),$(call build_stamp,$(os),$(1),$(2)))
GO_PACKAGES_NO_TMP=$(shell $(GOCMD) list ./... | grep -v '^github.com/dh-kam/pdf-go/tmp$$' | grep -v '^github.com/dh-kam/pdf-go/tmp/' | sed 's|^github.com/dh-kam/pdf-go|.|')
GO_PACKAGES_NO_TMP_NO_INTEG=$(shell echo "$(GO_PACKAGES_NO_TMP)" | tr ' ' '\n' | grep -v '^\./test/integration/pdf$$' | tr '\n' ' ')
GO_PACKAGES_NO_TMP_NO_E2E=$(shell echo "$(GO_PACKAGES_NO_TMP)" | tr ' ' '\n' | grep -v '^\./test/e2e$$' | tr '\n' ' ')
GOAL98_OUT?=$(CURDIR)/tmp/goal98_final_after_porting_complete_v2
GOAL98_PREV_REPORT?=$(CURDIR)/tmp/goal98_prev_report.csv
GOAL98_BASE_REPORT?=
GOAL98_CURRENT_REPORT?=$(GOAL98_OUT)/report.csv
GOAL98_FAIL_ON_REGRESSION?=0
GOAL98_THRESHOLD?=99
GOAL98_HTML_OUT?=$(GOAL98_OUT)/html
GOAL98_HTML_THRESHOLD?=99
GOAL98_IMAGE_SAMPLING_MODE?=legacy
SAMPLE_COMPARE_OUT?=$(CURDIR)/tmp/sample_compare
SAMPLE_COMPARE_THRESHOLD?=99
SAMPLE_COMPARE_DPI?=150
SAMPLE_COMPARE_TIMEOUT_SEC?=900
SAMPLE_COMPARE_SAMPLE_ROOT?=test/testdata/sample-files/
SAMPLE_COMPARE_SKIP_COMPRESSED_DUPLICATES?=false
SAMPLE_COMPARE_IMAGE_SAMPLING_MODE?=legacy
SAMPLE_COMPARE_TRADEOFF_OUT?=$(SAMPLE_COMPARE_OUT)/tradeoff.md
SAMPLE_COMPARE_TRADEOFF_PROFILE?=$(PROFILE_RENDER_OUT)/summary.log
SAMPLE_COMPARE_BACKLOG_OUT?=$(SAMPLE_COMPARE_OUT)/bottleneck_backlog.md
SAMPLE_COMPARE_BACKLOG_TOP_N?=8
SAMPLE_COMPARE_FOCUS_OUT?=$(SAMPLE_COMPARE_OUT)/image_sampling_focus.md
SAMPLE_COMPARE_FOCUS_FIXTURE?=test/testdata/image_sampling_focus.csv
SAMPLE_COMPARE_ICC_FOCUS_OUT?=$(CURDIR)/tmp/sample_compare_icc_focus
SAMPLE_COMPARE_ICC_FOCUS_MODES?=legacy,adaptive-dct-iccbased-v1
SAMPLE_COMPARE_ICC_FOCUS_TIMEOUT_SEC?=3600
SAMPLE_COMPARE_ICC_FOCUS_PER_PAGE_TIMEOUT_SEC?=600
SAMPLE_COMPARE_FAILDOCS_BASE?=$(SAMPLE_COMPARE_OUT)/report.csv
SAMPLE_COMPARE_FAILDOCS_LIST?=$(SAMPLE_COMPARE_OUT)/fail_docs.txt
SAMPLE_COMPARE_FAILDOCS_OUT?=$(CURDIR)/tmp/sample_compare_faildocs_recheck_only
SAMPLE_COMPARE_FAILDOCS_TIMEOUT_SEC?=3600
SAMPLE_COMPARE_FAILDOCS_PER_PAGE_TIMEOUT_SEC?=600
PROFILE_RENDER_OUT?=$(CURDIR)/tmp/prof_render
PROFILE_RENDER_DPI?=150
PROFILE_RENDER_WORKERS?=4
PROFILE_RENDER_ENABLE_CACHE?=false
PROFILE_RENDER_DOCS?=
PROFILE_RENDER_PAGES?=
PROFILE_RENDER_MAX_AVG_MS?=80
PROFILE_RENDER_MAX_SLOWEST_MS?=200
LEAK_CHECK_OUT?=$(CURDIR)/tmp/render_leak_check
LEAK_CHECK_DPI?=150
LEAK_CHECK_WORKERS?=4
LEAK_CHECK_DOCS?=
LEAK_CHECK_PAGES?=
LEAK_CHECK_ITERATIONS?=8
LEAK_CHECK_WARMUP?=2
LEAK_CHECK_ENABLE_CACHE?=false
LEAK_CHECK_MAX_HEAP_GROWTH_KB?=2048
NIGHTLY_PROFILE_OUT?=$(CURDIR)/tmp/nightly_profile
NIGHTLY_SAMPLE_OUT?=$(CURDIR)/tmp/nightly_sample_compare
NIGHTLY_PROFILE_DOCS?=test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf,test/testdata/sample-files/011-google-doc-document/google-doc-document.pdf,test/testdata/sample-files/018-base64-image/base64image.pdf,test/testdata/sample-files/023-cmyk-image/cmyk-image.pdf,test/testdata/sample-files/026-latex-multicolumn/multicolumn.pdf
NIGHTLY_PROFILE_PAGES?=1,59,117,1,1,1,1,1,2
NIGHTLY_BASE_SAMPLE_REPORT?=$(CURDIR)/tmp/sample_compare_faildocs_recheck_only/report.csv
NIGHTLY_FAIL_ON_REGRESSION?=0
COVERAGE_CORE_PROFILE?=coverage_no_cgo_core.txt
COVERAGE_CORE_SUMMARY?=tmp/coverage_core_no_cgo.txt
COVERAGE_CORE_MIN?=80.0
COVERAGE_CORE_EXCLUDE_REGEX?=github.com/dh-kam/pdf-go/pkg/pdf/|github.com/dh-kam/pdf-go/internal/infrastructure/font/
RELEASE_VERSION?=v0.9.0-202602.1
RELEASE_MODULE?=github.com/dh-kam/pdf-go/pkg/pdf
RELEASE_DRY_RUN?=true
PDFJS_SCAN_ROOT?=$(abspath $(CURDIR)/..)
PDFJS_ROOT?=$(PDFJS_SCAN_ROOT)/pdf.js
PDFJS_TEST_ROOT?=$(PDFJS_ROOT)/test
PDFJS_MANIFEST?=$(PDFJS_TEST_ROOT)/test_manifest.json
PDFJS_PARITY_OUT?=$(CURDIR)/tmp/pdfjs_parity
PDFJS_PARITY_DOC_LIST?=$(CURDIR)/tmp/pdfjs_parity_include_docs.txt
PDFJS_PARITY_LEGACY_GLOBS?=$(CURDIR)/tmp/pdfjs_parity2 $(CURDIR)/tmp/pdfjs_parity_run $(CURDIR)/tmp/pdfjs_parity_test_small $(CURDIR)/tmp/pdfjs_issue17147_pngcheck
PDFJS_PARITY_THRESHOLD?=98
PDFJS_PARITY_DPI?=150
PDFJS_PARITY_LIMIT?=3

.PHONY: $(OS_LIST) $(ARCH_LIST) $(BUILD_VARIANTS) $(OS_ARCH_PAIRS) $(OS_VARIANT_PAIRS) $(ARCH_VARIANT_PAIRS) $(FULL_SELECTOR_TARGETS)

define matrix_build_target
$(call build_stamp,$(1),$(2),$(3)): $(GO_FILES)
	@$$(MAKE) --no-print-directory build-all TARGET_OS=$(1) TARGET_ARCH=$(2) TARGET_VARIANT=$(3)
	@touch $$@
endef

$(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(eval $(call matrix_build_target,$(os),$(arch),$(var))))))

# Build every configured os/arch/variant combination.
all: $(FULL_BUILD_TARGETS)

$(OS_LIST):
	@$(MAKE) --no-print-directory $(call all_for_os,$(@F))

$(ARCH_LIST):
	@$(MAKE) --no-print-directory $(call all_for_arch,$(@F))

$(BUILD_VARIANTS):
	@$(MAKE) --no-print-directory $(call all_for_variant,$(@F))

$(OS_ARCH_PAIRS):
	@$(MAKE) --no-print-directory $(call all_for_os_arch,$(call token1,$(@F)),$(call token2,$(@F)))

$(OS_VARIANT_PAIRS):
	@$(MAKE) --no-print-directory $(call all_for_os_variant,$(call token1,$(@F)),$(call token2,$(@F)))

$(ARCH_VARIANT_PAIRS):
	@$(MAKE) --no-print-directory $(call all_for_arch_variant,$(call token1,$(@F)),$(call token2,$(@F)))

$(FULL_SELECTOR_TARGETS):
	@$(MAKE) --no-print-directory $(call build_stamp,$(call token1,$(@F)),$(call token2,$(@F)),$(call token3,$(@F)))

# Build the primary pdf-go application.
build: build-pdf-go

build-pdf-go:
	@echo "Building $(MAIN_BINARY_NAME) for $(TARGET_OS)/$(TARGET_ARCH)/$(TARGET_VARIANT)..."
	@mkdir -p "$(TARGET_BUILD_DIR)"
	CGO_ENABLED=$(TARGET_CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GOBUILD) $(TARGET_GOFLAGS) $(TARGET_TAG_FLAGS) -o "$(TARGET_BUILD_DIR)/$(MAIN_BINARY_NAME)$(TARGET_EXE)" $(CMD_DIR)/pdfrender
	@echo "Build complete: $(TARGET_BUILD_DIR)/$(MAIN_BINARY_NAME)$(TARGET_EXE)"

# Build pdfinfo tool.
build-pdfinfo:
	@echo "Building pdfinfo for $(TARGET_OS)/$(TARGET_ARCH)/$(TARGET_VARIANT)..."
	@mkdir -p "$(TARGET_BUILD_DIR)"
	CGO_ENABLED=$(TARGET_CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GOBUILD) $(TARGET_GOFLAGS) $(TARGET_TAG_FLAGS) -o "$(TARGET_BUILD_DIR)/pdfinfo$(TARGET_EXE)" $(CMD_DIR)/pdfinfo
	@echo "Build complete: $(TARGET_BUILD_DIR)/pdfinfo$(TARGET_EXE)"

# Build pdftext tool.
build-pdftext:
	@echo "Building pdftext for $(TARGET_OS)/$(TARGET_ARCH)/$(TARGET_VARIANT)..."
	@mkdir -p "$(TARGET_BUILD_DIR)"
	CGO_ENABLED=$(TARGET_CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GOBUILD) $(TARGET_GOFLAGS) $(TARGET_TAG_FLAGS) -o "$(TARGET_BUILD_DIR)/pdftext$(TARGET_EXE)" $(CMD_DIR)/pdftext
	@echo "Build complete: $(TARGET_BUILD_DIR)/pdftext$(TARGET_EXE)"

# Build pdfrender tool.
build-pdfrender:
	@echo "Building pdfrender for $(TARGET_OS)/$(TARGET_ARCH)/$(TARGET_VARIANT)..."
	@mkdir -p "$(TARGET_BUILD_DIR)"
	CGO_ENABLED=$(TARGET_CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GOBUILD) $(TARGET_GOFLAGS) $(TARGET_TAG_FLAGS) -o "$(TARGET_BUILD_DIR)/pdfrender$(TARGET_EXE)" $(CMD_DIR)/pdfrender
	@echo "Build complete: $(TARGET_BUILD_DIR)/pdfrender$(TARGET_EXE)"

# Build all cmd/* CLI tools and cmd/*.go ad-hoc tools.
build-cli:
	@echo "Building CLI tools into $(TARGET_BUILD_DIR)..."
	@mkdir -p "$(TARGET_BUILD_DIR)"
	@for dir in $(CLI_PACKAGE_DIRS); do \
		name=$$(basename "$$dir"); \
		echo "  $$name"; \
		CGO_ENABLED=$(TARGET_CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GOBUILD) $(TARGET_GOFLAGS) $(TARGET_TAG_FLAGS) -o "$(TARGET_BUILD_DIR)/$$name$(TARGET_EXE)" "$$dir"; \
	done
	@for src in $(CLI_FILE_SOURCES); do \
		name=$$(basename "$$src" .go); \
		echo "  $$name"; \
		CGO_ENABLED=$(TARGET_CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GOBUILD) $(TARGET_GOFLAGS) $(TARGET_TAG_FLAGS) -o "$(TARGET_BUILD_DIR)/$$name$(TARGET_EXE)" "$$src"; \
	done
	@echo "CLI build complete: $(TARGET_BUILD_DIR)"

# Build runnable examples.
build-examples:
	@echo "Building examples into $(TARGET_BUILD_DIR)..."
	@mkdir -p "$(TARGET_BUILD_DIR)"
	@if [ -z "$(EXAMPLE_SOURCES)" ]; then \
		echo "No example sources found."; \
	else \
		for src in $(EXAMPLE_SOURCES); do \
			name=$$(basename "$$src" .go); \
			echo "  $$name"; \
			CGO_ENABLED=$(TARGET_CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GOBUILD) $(TARGET_GOFLAGS) $(TARGET_TAG_FLAGS) -o "$(TARGET_BUILD_DIR)/$$name$(TARGET_EXE)" "$$src"; \
		done; \
	fi
	@echo "Example build complete: $(TARGET_BUILD_DIR)"

# Build pdf-go, all CLI tools, and examples for the selected target.
build-all: build-pdf-go build-cli build-examples
	@echo "Build complete: $(TARGET_BUILD_DIR)"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.txt $(GO_PACKAGES_NO_TMP)

# Run tests without race detector (faster)
test-fast:
	@echo "Running tests (fast)..."
	$(GOTEST) -v -coverprofile=coverage.txt $(GO_PACKAGES_NO_TMP)

# Run tests without CGo dependencies
test-no-cgo:
	@echo "Running tests (no CGo)..."
	$(GOTEST) -v -tags='nojpx,nojbig2' $(GO_PACKAGES_NO_TMP)

# Run race tests without CGo dependencies
test-no-cgo-race:
	@echo "Running race tests (no CGo, excluding long integration package)..."
	$(GOTEST) -v -race -timeout=30m -tags='nojpx,nojbig2' $(GO_PACKAGES_NO_TMP_NO_INTEG)

# Generate coverage report
coverage: test
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report: coverage.html"

# Show coverage in terminal
coverage-show: test
	@echo "Coverage summary:"
	$(GOCMD) tool cover -func=coverage.txt

# Generate and show coverage report without CGo dependencies
coverage-no-cgo:
	@echo "Running coverage (no CGo)..."
	$(GOTEST) -tags='nojpx,nojbig2' -coverpkg=./internal/...,./pkg/... -coverprofile=coverage_no_cgo.txt $(GO_PACKAGES_NO_TMP_NO_E2E)
	@TOTAL="$$( $(GOCMD) tool cover -func=coverage_no_cgo.txt | awk '/^total:/ {print $$3}' )"; \
	echo "Coverage total (no CGo): $$TOTAL"

# Generate core coverage metric (excluding compatibility alias/runtime shims and large parser-heavy font packages)
# and enforce minimum threshold.
coverage-core-no-cgo: coverage-no-cgo
	@echo "Running core coverage gate (no CGo)..."
	@awk -v re='$(COVERAGE_CORE_EXCLUDE_REGEX)' 'NR==1 || $$1 !~ re {print}' coverage_no_cgo.txt > "$(COVERAGE_CORE_PROFILE)"
	@$(GOCMD) tool cover -func="$(COVERAGE_CORE_PROFILE)" | tee "$(COVERAGE_CORE_SUMMARY)"
	@TOTAL="$$(awk '/^total:/ {gsub("%","",$$3); print $$3}' "$(COVERAGE_CORE_SUMMARY)")"; \
	echo "Coverage core total (no CGo): $${TOTAL}% (min $(COVERAGE_CORE_MIN)%)"; \
	awk -v total="$$TOTAL" -v min="$(COVERAGE_CORE_MIN)" 'BEGIN { if ((total+0) < (min+0)) { printf("Coverage core is below %.1f%%\n", min); exit 1 } }'

# Run render regression exact-compare tests without CGo dependencies
render-regression-no-cgo:
	@echo "Running render regression tests (no CGo)..."
	$(GOTEST) -v -count=1 -tags='nojpx,nojbig2' ./test/e2e -run TestCLI_PDFRender_ExactBaseline

# Run linter
lint:
	@echo "Running linter..."
	$(GOLINT) run $(GO_PACKAGES_NO_TMP)

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	$(GOCMD) fmt ./...

# Generate mocks
mock-gen:
	@echo "Generating mocks..."
	$(GOCMD) generate ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR) $(BUILD_ROOT)
	rm -f coverage.txt coverage.html
	rm -f test/mock/*.go

# Run tests with coverage check (fail if < 80%)
test-coverage: test
	@echo "Checking coverage..."
	$(GOCMD) tool cover -func=coverage.txt | grep total | awk '{if ($$3+0 < 80.0) { print "Coverage is below 80%"; exit 1 }}'

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem $(GO_PACKAGES_NO_TMP)

# Run vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet $(GO_PACKAGES_NO_TMP)

# Run vet without CGo dependencies
vet-no-cgo:
	@echo "Running go vet (no CGo)..."
	$(GOCMD) vet -tags='nojpx,nojbig2' $(GO_PACKAGES_NO_TMP)

# Build without CGo (minimal dependencies)
build-pure:
	@echo "Building without CGo..."
	@$(MAKE) --no-print-directory build-pdf-go TARGET_VARIANT=nocgo TARGET_CGO_ENABLED=0 TARGET_TAGS="$(NO_CGO_TAGS)"

# Build all CLI tools with full CGo support
build-all-cgo:
	@echo "Building all CLI tools with full CGo support..."
	@$(MAKE) --no-print-directory build-all TARGET_VARIANT=default TARGET_CGO_ENABLED=1 TARGET_TAGS=
	@echo "Build complete (with CGo support)"

# Build all CLI tools without CGo
build-no-cgo:
	@echo "Building all CLI tools without CGo..."
	@$(MAKE) --no-print-directory build-all TARGET_VARIANT=nocgo TARGET_CGO_ENABLED=0 TARGET_TAGS="$(NO_CGO_TAGS)"
	@mkdir -p $(BUILD_DIR)
	@for name in $(CORE_CLI_TOOLS); do \
		echo "  legacy $(BUILD_DIR)/$$name$(HOST_EXE)"; \
		CGO_ENABLED=0 GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) $(GOBUILD) $(GO_DEFAULT_FLAGS) -tags='$(NO_CGO_TAGS)' -o "$(BUILD_DIR)/$$name$(HOST_EXE)" "$(CMD_DIR)/$$name"; \
	done
	@echo "Build complete (without CGo support)"

# Install development tools
install-tools:
	@echo "Installing development tools..."
	$(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint
	$(GOGET) -u github.com/golang/mock/mockgen
	$(GOGET) -u golang.org/x/tools/cmd/goimports

# Run all checks
check: fmt vet lint test-coverage
	@echo "All checks passed!"

# Run no-CGo validation checks
check-no-cgo: vet-no-cgo test-no-cgo-race coverage-core-no-cgo render-regression-no-cgo
	@echo "No-CGo checks passed!"

# Run integration parity report against poppler and write CSV/summary artifacts.
render-parity-report-test:
	@echo "Running render parity report test..."
	$(GOTEST) -v ./test/integration/pdf -run TestRenderParityReportAgainstPoppler -count=1

# Run lex fixture parity report against poppler and write CSV/summary artifacts.
lex-render-parity-report-test:
	@echo "Running lex render parity report test..."
	$(GOTEST) -v ./test/integration/pdf -run TestLexRenderParityAgainstPoppler -count=1

# Print worst render parity pages/documents from report.csv for prioritization.
render-parity-priority:
	@echo "Summarizing render parity priorities..."
	python3 ./scripts/render_parity_priority.py

# Run vulnerability scan without CGo dependencies
vuln-no-cgo:
	@echo "Running vulnerability scan (no CGo)..."
	$(GOCMD) run golang.org/x/vuln/cmd/govulncheck@latest -tags='nojpx,nojbig2' $(GO_PACKAGES_NO_TMP)

# Run no-CGo CLI smoke checks
cli-smoke-no-cgo: build-no-cgo
	@echo "Running no-CGo CLI smoke checks..."
	./bin/pdfinfo -h >/tmp/pdfinfo_help.txt 2>&1
	./bin/pdftext -h >/tmp/pdftext_help.txt 2>&1
	./bin/pdfrender -h >/tmp/pdfrender_help.txt 2>&1
	@echo "No-CGo CLI smoke checks passed!"

# Run full PNG parity batch against poppler (goal98 profile)
goal98-batch-no-cgo:
	@echo "Running goal98 batch (no CGo)..."
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/goal98_batch.go \
		-repo-root $(CURDIR) \
		-scan-root $(CURDIR) \
		-out $(GOAL98_OUT) \
		-dpi 150 \
		-image-sampling-mode $(GOAL98_IMAGE_SAMPLING_MODE) \
		-threshold $(GOAL98_THRESHOLD) \
		-threshold-mae $(GOAL98_THRESHOLD) \
		-timeout-sec 900

# Run goal98 batch unit tests without CGo.
goal98-batch-test-no-cgo:
	@echo "Running goal98 batch unit tests (no CGo)..."
	$(GOCMD) test -tags='nojpx,nojbig2' ./tmp/goal98_batch.go ./tmp/goal98_batch_test.go -count=1

# Generate goal98 HTML compare report (poppler vs ours vs xor).
goal98-html-no-cgo:
	@echo "Generating goal98 HTML compare report (no CGo)..."
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/goal98_compare_html.go \
		-report $(GOAL98_OUT)/report.csv \
		-out $(GOAL98_HTML_OUT) \
		-threshold $(GOAL98_HTML_THRESHOLD) \
		-sample-only=true

# Run sample-only poppler-vs-ours compare and generate HTML report (99% gate).
sample-compare-html-no-cgo:
	@echo "Running sample-only compare batch + HTML report (no CGo)..."
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/goal98_batch.go \
		-repo-root $(CURDIR) \
		-scan-root $(CURDIR) \
		-out $(SAMPLE_COMPARE_OUT) \
		-dpi $(SAMPLE_COMPARE_DPI) \
		-image-sampling-mode $(SAMPLE_COMPARE_IMAGE_SAMPLING_MODE) \
		-threshold $(SAMPLE_COMPARE_THRESHOLD) \
		-threshold-mae $(SAMPLE_COMPARE_THRESHOLD) \
		-timeout-sec $(SAMPLE_COMPARE_TIMEOUT_SEC) \
		-sample-only=true \
		-sample-root $(SAMPLE_COMPARE_SAMPLE_ROOT) \
		-skip-compressed-duplicates=$(SAMPLE_COMPARE_SKIP_COMPRESSED_DUPLICATES)
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/goal98_compare_html.go \
		-report $(SAMPLE_COMPARE_OUT)/report.csv \
		-out $(SAMPLE_COMPARE_OUT)/html \
		-threshold $(SAMPLE_COMPARE_THRESHOLD) \
		-sample-only=true \
		-sample-root $(SAMPLE_COMPARE_SAMPLE_ROOT)
	@echo "Sample report CSV: $(SAMPLE_COMPARE_OUT)/report.csv"
	@echo "Sample compare HTML: $(SAMPLE_COMPARE_OUT)/html/index.html"

# Generate sample compare tradeoff markdown report (accuracy + performance).
sample-compare-tradeoff-no-cgo:
	@echo "Generating sample compare tradeoff report..."
	@./scripts/render_tradeoff_report.sh \
		"$(SAMPLE_COMPARE_OUT)/report.csv" \
		"$(SAMPLE_COMPARE_TRADEOFF_OUT)" \
		"$(SAMPLE_COMPARE_TRADEOFF_PROFILE)"
	@echo "Sample tradeoff report: $(SAMPLE_COMPARE_TRADEOFF_OUT)"

# Generate stakeholder requirements + prioritized backlog from profile and compare outputs.
sample-compare-backlog-no-cgo:
	@echo "Generating sample compare bottleneck backlog report..."
	@./scripts/render_bottleneck_backlog.sh \
		"$(SAMPLE_COMPARE_TRADEOFF_PROFILE)" \
		"$(dir $(SAMPLE_COMPARE_TRADEOFF_PROFILE))/cpu.out" \
		"$(dir $(SAMPLE_COMPARE_TRADEOFF_PROFILE))/mem.out" \
		"$(SAMPLE_COMPARE_OUT)/report.csv" \
		"$(SAMPLE_COMPARE_BACKLOG_OUT)" \
		"$(SAMPLE_COMPARE_BACKLOG_TOP_N)"
	@echo "Sample bottleneck backlog report: $(SAMPLE_COMPARE_BACKLOG_OUT)"

# Generate focused page-level report for key image sampling bottlenecks.
sample-compare-focus-no-cgo:
	@echo "Generating image sampling focus report..."
	@./scripts/render_image_sampling_focus.sh \
		"$(SAMPLE_COMPARE_OUT)/report.csv" \
		"$(SAMPLE_COMPARE_FOCUS_OUT)" \
		"$(SAMPLE_COMPARE_FOCUS_FIXTURE)"
	@echo "Sample focus report: $(SAMPLE_COMPARE_FOCUS_OUT)"

# Compare ICC/gray/cmyk focus docs across image sampling modes and generate matrix report.
sample-compare-iccbased-focus-no-cgo:
	@echo "Running ICCBased focus matrix compare..."
	@./scripts/render_iccbased_focus_matrix.sh \
		"$(CURDIR)" \
		"$(SAMPLE_COMPARE_ICC_FOCUS_OUT)" \
		"$(SAMPLE_COMPARE_ICC_FOCUS_MODES)" \
		"$(SAMPLE_COMPARE_ICC_FOCUS_TIMEOUT_SEC)" \
		"$(SAMPLE_COMPARE_ICC_FOCUS_PER_PAGE_TIMEOUT_SEC)" \
		"$(CURDIR)/$(SAMPLE_COMPARE_FOCUS_FIXTURE)" \
		"$(SAMPLE_COMPARE_SAMPLE_ROOT)"
	@echo "ICCBased focus matrix: $(SAMPLE_COMPARE_ICC_FOCUS_OUT)/matrix.md"

# Re-run sample compare only for docs that failed in baseline report and regenerate HTMLs.
sample-compare-faildocs-recheck-no-cgo:
	@echo "Generating failed-doc list from baseline report..."
	@awk -F, 'NR>1 && ($$6 != "true" || $$7 != "true" || $$10 != "") {print $$1}' "$(SAMPLE_COMPARE_FAILDOCS_BASE)" \
		| sort -u > "$(SAMPLE_COMPARE_FAILDOCS_LIST)"
	@echo "Failed docs: $$(wc -l < "$(SAMPLE_COMPARE_FAILDOCS_LIST)")"
	@echo "Running failed-doc recompare batch..."
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/goal98_batch.go \
		-repo-root $(CURDIR) \
		-scan-root $(CURDIR) \
		-out $(SAMPLE_COMPARE_FAILDOCS_OUT) \
		-dpi $(SAMPLE_COMPARE_DPI) \
		-image-sampling-mode $(SAMPLE_COMPARE_IMAGE_SAMPLING_MODE) \
		-threshold $(SAMPLE_COMPARE_THRESHOLD) \
		-threshold-mae $(SAMPLE_COMPARE_THRESHOLD) \
		-timeout-sec $(SAMPLE_COMPARE_FAILDOCS_TIMEOUT_SEC) \
		-per-page-timeout-sec $(SAMPLE_COMPARE_FAILDOCS_PER_PAGE_TIMEOUT_SEC) \
		-sample-only=true \
		-sample-root $(SAMPLE_COMPARE_SAMPLE_ROOT) \
		-skip-compressed-duplicates=$(SAMPLE_COMPARE_SKIP_COMPRESSED_DUPLICATES) \
		-include-doc-list $(SAMPLE_COMPARE_FAILDOCS_LIST)
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/goal98_compare_html.go \
		-report $(SAMPLE_COMPARE_FAILDOCS_OUT)/report.csv \
		-out $(SAMPLE_COMPARE_FAILDOCS_OUT)/html \
		-threshold $(SAMPLE_COMPARE_THRESHOLD) \
		-sample-only=true \
		-sample-root $(SAMPLE_COMPARE_SAMPLE_ROOT)
	@awk -F, 'NR==1 || $$6 != "true" || $$7 != "true" || $$10 != ""' \
		"$(SAMPLE_COMPARE_FAILDOCS_OUT)/report.csv" > "$(SAMPLE_COMPARE_FAILDOCS_OUT)/report_fail_only.csv"
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/goal98_compare_html.go \
		-report $(SAMPLE_COMPARE_FAILDOCS_OUT)/report_fail_only.csv \
		-out $(SAMPLE_COMPARE_FAILDOCS_OUT)/html_fail_only \
		-threshold $(SAMPLE_COMPARE_THRESHOLD) \
		-sample-only=true \
		-sample-root $(SAMPLE_COMPARE_SAMPLE_ROOT)
	@echo "Fail-doc list:   $(SAMPLE_COMPARE_FAILDOCS_LIST)"
	@echo "Fail-doc report: $(SAMPLE_COMPARE_FAILDOCS_OUT)/report.csv"
	@echo "HTML report:     $(SAMPLE_COMPARE_FAILDOCS_OUT)/html/index.html"
	@echo "Fail-only HTML:  $(SAMPLE_COMPARE_FAILDOCS_OUT)/html_fail_only/index.html"

# Remove stale pdf.js parity outputs and legacy scratch directories.
pdfjs-parity-clean:
	@echo "Cleaning pdf.js parity outputs..."
	rm -rf "$(PDFJS_PARITY_OUT)" "$(PDFJS_PARITY_DOC_LIST)" $(PDFJS_PARITY_LEGACY_GLOBS)

# Select local whole-page eq fixtures from pdf.js test manifest.
pdfjs-select-eq: pdfjs-parity-clean
	@echo "Selecting local whole-page eq fixtures from pdf.js manifest..."
	@mkdir -p "$(PDFJS_PARITY_OUT)"
	$(GOCMD) run ./tmp/pdfjs_manifest_select.go \
		-manifest "$(PDFJS_MANIFEST)" \
		-scan-root "$(PDFJS_SCAN_ROOT)" \
		-test-root "$(PDFJS_TEST_ROOT)" \
		-out "$(PDFJS_PARITY_DOC_LIST)" \
		-type eq \
		-whole-pages-only=true \
		-limit "$(PDFJS_PARITY_LIMIT)"
	@echo "Selected docs list: $(PDFJS_PARITY_DOC_LIST)"

# Run a fast render parity batch driven by pdf.js test_manifest eq cases.
pdfjs-render-parity: pdfjs-select-eq
	@echo "Running pdf.js manifest render parity batch..."
	$(GOCMD) run ./tmp/goal98_batch.go \
		-repo-root "$(CURDIR)" \
		-scan-root "$(PDFJS_SCAN_ROOT)" \
		-out "$(PDFJS_PARITY_OUT)" \
		-dpi "$(PDFJS_PARITY_DPI)" \
		-threshold "$(PDFJS_PARITY_THRESHOLD)" \
		-threshold-mae "$(PDFJS_PARITY_THRESHOLD)" \
		-timeout-sec 900 \
		-include-doc-list "$(PDFJS_PARITY_DOC_LIST)"
	$(GOCMD) run ./tmp/goal98_compare_html.go \
		-report "$(PDFJS_PARITY_OUT)/report.csv" \
		-out "$(PDFJS_PARITY_OUT)/html" \
		-threshold "$(PDFJS_PARITY_THRESHOLD)" \
		-sample-root "pdf.js/test/pdfs/"
	@echo "pdf.js parity report: $(PDFJS_PARITY_OUT)/report.csv"
	@echo "pdf.js parity HTML:   $(PDFJS_PARITY_OUT)/html/index.html"

# Run nightly performance snapshot workflow locally (profile + sample compare + tradeoff report).
nightly-snapshot-no-cgo:
	@echo "Running nightly snapshot pipeline (no CGo)..."
	@$(MAKE) profile-render-guard-no-cgo \
		PROFILE_RENDER_OUT="$(NIGHTLY_PROFILE_OUT)" \
		PROFILE_RENDER_DOCS="$(NIGHTLY_PROFILE_DOCS)" \
		PROFILE_RENDER_PAGES="$(NIGHTLY_PROFILE_PAGES)"
	@$(MAKE) sample-compare-html-no-cgo \
		SAMPLE_COMPARE_OUT="$(NIGHTLY_SAMPLE_OUT)" \
		SAMPLE_COMPARE_SKIP_COMPRESSED_DUPLICATES=true
	@$(MAKE) sample-compare-tradeoff-no-cgo \
		SAMPLE_COMPARE_OUT="$(NIGHTLY_SAMPLE_OUT)" \
		SAMPLE_COMPARE_TRADEOFF_PROFILE="$(NIGHTLY_PROFILE_OUT)/summary.log"
	@$(MAKE) sample-compare-backlog-no-cgo \
		SAMPLE_COMPARE_OUT="$(NIGHTLY_SAMPLE_OUT)" \
		SAMPLE_COMPARE_TRADEOFF_PROFILE="$(NIGHTLY_PROFILE_OUT)/summary.log"
	@$(MAKE) sample-compare-focus-no-cgo \
		SAMPLE_COMPARE_OUT="$(NIGHTLY_SAMPLE_OUT)"
	@$(MAKE) nightly-compare-diff-no-cgo \
		NIGHTLY_SAMPLE_OUT="$(NIGHTLY_SAMPLE_OUT)" \
		NIGHTLY_BASE_SAMPLE_REPORT="$(NIGHTLY_BASE_SAMPLE_REPORT)" \
		NIGHTLY_FAIL_ON_REGRESSION="$(NIGHTLY_FAIL_ON_REGRESSION)"
	@echo "Nightly profile summary: $(NIGHTLY_PROFILE_OUT)/summary.log"
	@echo "Nightly compare report:  $(NIGHTLY_SAMPLE_OUT)/html/index.html"
	@echo "Nightly tradeoff report: $(NIGHTLY_SAMPLE_OUT)/tradeoff.md"
	@echo "Nightly backlog report:  $(NIGHTLY_SAMPLE_OUT)/bottleneck_backlog.md"
	@echo "Nightly focus report:    $(NIGHTLY_SAMPLE_OUT)/image_sampling_focus.md"
	@echo "Nightly diff report:     $(NIGHTLY_SAMPLE_OUT)/report_diff.txt"

# Compare nightly sample report against baseline and emit regression warning/failure.
nightly-compare-diff-no-cgo:
	@mkdir -p "$(NIGHTLY_SAMPLE_OUT)"
	@if [ -z "$(NIGHTLY_BASE_SAMPLE_REPORT)" ] || [ ! -f "$(NIGHTLY_BASE_SAMPLE_REPORT)" ]; then \
		echo "Skipping nightly compare diff (baseline missing): $(NIGHTLY_BASE_SAMPLE_REPORT)" | tee "$(NIGHTLY_SAMPLE_OUT)/report_diff.txt"; \
	else \
		if [ "$(NIGHTLY_FAIL_ON_REGRESSION)" = "1" ]; then \
			./scripts/goal98_report_diff.sh "$(NIGHTLY_BASE_SAMPLE_REPORT)" "$(NIGHTLY_SAMPLE_OUT)/report.csv" --fail-on-regression | tee "$(NIGHTLY_SAMPLE_OUT)/report_diff.txt"; \
		else \
			./scripts/goal98_report_diff.sh "$(NIGHTLY_BASE_SAMPLE_REPORT)" "$(NIGHTLY_SAMPLE_OUT)/report.csv" | tee "$(NIGHTLY_SAMPLE_OUT)/report_diff.txt"; \
		fi; \
	fi
	@echo "Nightly compare diff report: $(NIGHTLY_SAMPLE_OUT)/report_diff.txt"

# Collect rendering CPU/heap profiles on representative samples.
profile-render-no-cgo:
	@echo "Profiling renderer (no CGo)..."
	@mkdir -p $(PROFILE_RENDER_OUT)
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/render_profile.go \
		-root $(CURDIR) \
		-dpi $(PROFILE_RENDER_DPI) \
		-workers $(PROFILE_RENDER_WORKERS) \
		-enable-cache=$(PROFILE_RENDER_ENABLE_CACHE) \
		-cpu-profile $(PROFILE_RENDER_OUT)/cpu.out \
		-mem-profile $(PROFILE_RENDER_OUT)/mem.out \
		$(if $(PROFILE_RENDER_DOCS),-docs $(PROFILE_RENDER_DOCS),) \
		$(if $(PROFILE_RENDER_PAGES),-pages $(PROFILE_RENDER_PAGES),) \
		| tee $(PROFILE_RENDER_OUT)/summary.log
	@echo "Profile summary: $(PROFILE_RENDER_OUT)/summary.log"
	@echo "CPU profile:      $(PROFILE_RENDER_OUT)/cpu.out"
	@echo "Heap profile:     $(PROFILE_RENDER_OUT)/mem.out"

# Run render profile and fail on latency regressions.
profile-render-guard-no-cgo: profile-render-no-cgo
	@./scripts/profile_render_guard.sh \
		"$(PROFILE_RENDER_OUT)/summary.log" \
		"$(PROFILE_RENDER_MAX_AVG_MS)" \
		"$(PROFILE_RENDER_MAX_SLOWEST_MS)"

# Run repeated render iterations and fail when heap growth exceeds threshold.
render-leak-check-no-cgo:
	@echo "Running renderer leak check (no CGo)..."
	@mkdir -p $(LEAK_CHECK_OUT)
	$(GOCMD) run -tags='nojpx,nojbig2' ./tmp/render_leak_check.go \
		-root $(CURDIR) \
		-dpi $(LEAK_CHECK_DPI) \
		-workers $(LEAK_CHECK_WORKERS) \
		-enable-cache=$(LEAK_CHECK_ENABLE_CACHE) \
		-iterations $(LEAK_CHECK_ITERATIONS) \
		-warmup $(LEAK_CHECK_WARMUP) \
		-max-heap-growth-kb $(LEAK_CHECK_MAX_HEAP_GROWTH_KB) \
		-out $(LEAK_CHECK_OUT)/summary.log \
		$(if $(LEAK_CHECK_DOCS),-docs $(LEAK_CHECK_DOCS),) \
		$(if $(LEAK_CHECK_PAGES),-pages $(LEAK_CHECK_PAGES),)
	@echo "Leak check summary: $(LEAK_CHECK_OUT)/summary.log"

# Compare two goal98 CSV reports
goal98-compare-report:
	@echo "Comparing goal98 reports..."
	@if [ -z "$(GOAL98_BASE_REPORT)" ]; then \
		echo "GOAL98_BASE_REPORT is required"; \
		exit 1; \
	fi
	@if [ "$(GOAL98_FAIL_ON_REGRESSION)" = "1" ]; then \
		./scripts/goal98_report_diff.sh "$(GOAL98_BASE_REPORT)" "$(GOAL98_CURRENT_REPORT)" --fail-on-regression; \
	else \
		./scripts/goal98_report_diff.sh "$(GOAL98_BASE_REPORT)" "$(GOAL98_CURRENT_REPORT)"; \
	fi

# Run goal98 batch and compare against previous report in the same output path
goal98-batch-compare-no-cgo:
	@echo "Running goal98 batch and comparing with previous report..."
	@if [ -f "$(GOAL98_OUT)/report.csv" ]; then \
		cp "$(GOAL98_OUT)/report.csv" "$(GOAL98_PREV_REPORT)"; \
		echo "Saved previous report: $(GOAL98_PREV_REPORT)"; \
	else \
		rm -f "$(GOAL98_PREV_REPORT)"; \
		echo "No previous report found at $(GOAL98_OUT)/report.csv"; \
	fi
	@$(MAKE) goal98-batch-no-cgo
	@if [ -f "$(GOAL98_PREV_REPORT)" ]; then \
		if [ "$(GOAL98_FAIL_ON_REGRESSION)" = "1" ]; then \
			./scripts/goal98_report_diff.sh "$(GOAL98_PREV_REPORT)" "$(GOAL98_OUT)/report.csv" --fail-on-regression; \
		else \
			./scripts/goal98_report_diff.sh "$(GOAL98_PREV_REPORT)" "$(GOAL98_OUT)/report.csv"; \
		fi; \
	else \
		echo "Skipped diff: previous report not available"; \
	fi

# Run goal98 batch compare and fail on regressions
goal98-guard-no-cgo:
	@$(MAKE) goal98-batch-test-no-cgo
	@$(MAKE) goal98-batch-compare-no-cgo GOAL98_FAIL_ON_REGRESSION=1

# Run full porting completion checks
porting-complete: lint check-no-cgo vuln-no-cgo cli-smoke-no-cgo
	@echo "Porting completion checks passed!"

# Run full porting completion checks and goal98 regression guard
porting-complete-plus-goal98: porting-complete goal98-guard-no-cgo
	@echo "Porting completion + goal98 guard checks passed!"

# Run release preflight checks before tagging/publishing.
release-preflight:
	@./scripts/release_preflight.sh "$(CURDIR)" "$(RELEASE_VERSION)"

# Run full release flow in dry-run mode.
release-dry-run:
	@DRY_RUN="$(RELEASE_DRY_RUN)" REPO_MODULE="$(RELEASE_MODULE)" ./scripts/release_publish.sh "$(CURDIR)" "$(RELEASE_VERSION)"

# Run full release flow (tag + push + GitHub release + module check).
release-publish:
	@DRY_RUN=false REPO_MODULE="$(RELEASE_MODULE)" ./scripts/release_publish.sh "$(CURDIR)" "$(RELEASE_VERSION)"

# Help
help:
	@echo "Available targets:"
	@echo "  all            - Build every OS/ARCH/VARIANT combination into $(BUILD_ROOT)/<os>-<arch>/<variant>/"
	@echo "  <os>|<arch>|<variant>|<os>-<arch>|<os>-<variant>|<arch>-<variant>|<os>-<arch>-<variant>"
	@echo "                 - Build selector combinations; OS=$(OS_LIST), ARCH=$(ARCH_LIST), VARIANT=$(BUILD_VARIANTS)"
	@echo "  build          - Build the main application as $(TARGET_BUILD_DIR)/$(MAIN_BINARY_NAME)$(TARGET_EXE)"
	@echo "  build-pdf-go   - Build the primary pdf-go application"
	@echo "  build-cli      - Build all cmd/* and cmd/*.go CLI tools into $(TARGET_BUILD_DIR)"
	@echo "  build-examples - Build runnable examples into $(TARGET_BUILD_DIR)"
	@echo "  build-pdfinfo  - Build pdfinfo tool into $(TARGET_BUILD_DIR)"
	@echo "  build-pdftext  - Build pdftext tool into $(TARGET_BUILD_DIR)"
	@echo "  build-pdfrender - Build pdfrender tool into $(TARGET_BUILD_DIR)"
	@echo "  build-all      - Build pdf-go, CLI tools, and examples into $(TARGET_BUILD_DIR)"
	@echo "  build-pure     - Build pdf-go without CGo using the nocgo variant"
	@echo "  build-all-cgo  - Build all tools with full CGo support using the default variant"
	@echo "  build-no-cgo   - Build all tools without CGo using the nocgo variant and sync legacy bin/ tools"
	@echo "  test           - Run tests with race detector"
	@echo "  test-fast      - Run tests without race detector"
	@echo "  coverage       - Generate HTML coverage report"
	@echo "  coverage-show  - Show coverage in terminal"
	@echo "  render-regression-no-cgo - Run no-CGo render baseline exact-compare regression tests"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"
	@echo "  mock-gen       - Generate mocks"
	@echo "  clean          - Clean build artifacts"
	@echo "  test-coverage  - Run tests with coverage check (fail if < 80%)"
	@echo "  deps           - Download dependencies"
	@echo "  bench          - Run benchmarks"
	@echo "  vet            - Run go vet"
	@echo "  install-tools  - Install development tools"
	@echo "  check          - Run all checks"
	@echo "  vuln-no-cgo    - Run vulnerability scan without CGo dependencies"
	@echo "  cli-smoke-no-cgo - Run no-CGo CLI smoke checks"
	@echo "  render-parity-report-test - Compare all scanned PDFs against poppler and write report artifacts"
	@echo "  lex-render-parity-report-test - Compare lex fixtures against poppler and classify confirmed/unconfirmed"
	@echo "  render-parity-priority - Print worst render parity pages/documents from report.csv"
	@echo "  goal98-batch-no-cgo - Run full PNG parity batch against poppler"
	@echo "  goal98-batch-test-no-cgo - Run goal98 batch unit tests without CGo"
	@echo "  goal98-html-no-cgo - Generate goal98 HTML compare report (poppler/ours/xor)"
	@echo "  pdfjs-select-eq - Select local whole-page eq docs from pdf.js test_manifest"
	@echo "  pdfjs-render-parity - Run fast pdf.js-manifest-driven poppler parity batch"
	@echo "  sample-compare-html-no-cgo - Run sample-only compare and generate HTML report (99% PASS)"
	@echo "  sample-compare-tradeoff-no-cgo - Generate sample compare tradeoff markdown report"
	@echo "  sample-compare-backlog-no-cgo - Generate stakeholder requirements/prioritized backlog report"
	@echo "  sample-compare-focus-no-cgo - Generate focused image-sampling page report"
	@echo "  sample-compare-iccbased-focus-no-cgo - Compare ICCBased focus docs across sampling modes"
	@echo "  sample-compare-faildocs-recheck-no-cgo - Re-run only failed docs from baseline report and regenerate HTMLs"
	@echo "  nightly-snapshot-no-cgo - Run nightly profile+compare+tradeoff+backlog+focus pipeline"
	@echo "  nightly-compare-diff-no-cgo - Diff nightly sample report against baseline and write report_diff.txt"
	@echo "  profile-render-no-cgo - Collect renderer CPU/heap profiles on sample PDFs"
	@echo "  profile-render-guard-no-cgo - Run profile and fail on latency regressions"
	@echo "  render-leak-check-no-cgo - Run repeated render leak check with heap-growth guard"
	@echo "  coverage-core-no-cgo - Run no-CGo coverage and enforce core threshold (default 80%)"
	@echo "  goal98-compare-report - Compare two goal98 CSV reports"
	@echo "  goal98-batch-compare-no-cgo - Run goal98 batch and compare with previous report"
	@echo "  goal98-guard-no-cgo - Run goal98 batch compare and fail on regressions"
	@echo "  porting-complete - Run full porting completion checks"
	@echo "  porting-complete-plus-goal98 - Run full porting + goal98 regression guard"
	@echo "  release-preflight - Validate git/auth/release gate before publishing"
	@echo "  release-dry-run - Simulate release steps without side effects"
	@echo "  release-publish - Execute release steps (tag/push/gh/module)"
	@echo "  splash-golden-snapshot - Capture splash golden PNG snapshots (SPLASH_GOLDEN_SNAPSHOT=1)"
	@echo "  splash-golden-verify   - Verify splash golden snapshots are stable across reruns"
	@echo "  splash-phase0-gate     - Phase 0 byte-identity gate (SPLASH_BACKEND=0 must match main CSV)"
	@echo "  help           - Show this help"

# === Splash port — Phase 0 ===

.PHONY: splash-golden-snapshot splash-golden-verify splash-phase0-gate

splash-golden-snapshot:
	@echo ">>> Producing splash golden snapshots (SPLASH_GOLDEN_SNAPSHOT=1)"
	SPLASH_GOLDEN_SNAPSHOT=1 SPLASH_BACKEND=1 \
	  $(GOTEST) ./test/integration/splash/ -run TestSplashGoldenCorpus -v -count=1

splash-golden-verify:
	@echo ">>> Verifying splash golden snapshots stable across reruns"
	SPLASH_BACKEND=1 \
	  $(GOTEST) ./test/integration/splash/ -run TestSplashGoldenCorpus -v -count=1

splash-phase0-gate:
	@echo ">>> Phase 0 byte-identity gate: SPLASH_BACKEND=0 must match main CSV"
	SPLASH_PHASE0_GATE=1 \
	  $(GOTEST) ./test/integration/splash/ -run TestPhase0CSVByteIdentity -v -count=1 -timeout=30m

# === Splash port — Phase 1 watch-set gate ===
.PHONY: splash-watchset

splash-watchset:
	@echo ">>> Splash watch-set non-regression gate (8 hardest pages)"
	$(GOTEST) ./test/integration/splash/ -run TestSplashWatchSet -v -count=1 -timeout=15m

# === Splash port — Phase 2 stroke + fill parity gate ===
.PHONY: splash-stroke-parity splash-watchset-probe

splash-stroke-parity:
	@echo ">>> Splash stroke + fill primitive parity (9 fixtures byte-equal vs pdftoppm 24.02.0)"
	$(GOTEST) ./test/integration/splash/ -run TestSplashStrokePrimitiveParity -v -count=1 -timeout=15m

splash-watchset-probe:
	@echo ">>> Splash watch-set diagnostic probe (8 pages, captures status+bytes)"
	$(GOTEST) ./test/integration/splash/ -run TestSplashWatchSetProbe -v -count=1 -timeout=20m

# === Splash port — Phase 3 corpus exact100 exit gate ===
.PHONY: splash-corpus-p3

splash-corpus-p3:
	@echo ">>> Phase 3 corpus exact100 measurement (heavy, 30-60 min)"
	SPLASH_CORPUS_RUN=1 \
	  $(GOTEST) ./test/integration/splash/ -run TestSplashCorpusMeasurement -v -count=1 -timeout=120m

# === Splash port — Phase 5 performance benchmark suite (R8/R9 mitigation) ===
.PHONY: splash-bench splash-bench-compare

splash-bench:
	@echo ">>> Splash benchmark suite (10 iterations, ns/op + B/op + allocs/op)"
	@mkdir -p tmp
	$(GOTEST) -bench=. -benchmem -count=10 ./internal/infrastructure/splash/... | tee tmp/splash_bench.txt

splash-bench-compare:
	@echo ">>> Splash benchmarks vs baseline.txt (requires benchstat)"
	bash scripts/splash_bench_compare.sh baseline.txt

# === Splash port — Phase 5 tier-1 (PR-time) + tier-2 (nightly) corpus gates ===
.PHONY: splash-corpus-tier1 splash-corpus-tier2

splash-corpus-tier1:
	@echo ">>> Splash corpus tier-1 (PR-time, ~30 pages, ~90s)"
	$(GOTEST) ./test/integration/splash/ -run TestSplashCorpusTier1 -v -count=1 -timeout=3m

splash-corpus-tier2:
	@echo ">>> Splash corpus tier-2 full 286-page matrix (nightly, ~5 min)"
	SPLASH_CORPUS_RUN=1 \
	  $(GOTEST) ./test/integration/splash/ -run TestSplashCorpusMeasurement -v -count=1 -timeout=120m
