TESTDATA_REPO  := o3co/xx.hocon
TESTDATA_REF   := main
EXPECTED_DIR   := testdata/expected

.PHONY: testdata test

testdata:
	@if [ -f .xx-hocon-version ] && [ -d "$(EXPECTED_DIR)" ]; then \
	  remote_sha=$$(curl -sf "https://api.github.com/repos/$(TESTDATA_REPO)/commits/$(TESTDATA_REF)" | grep '"sha"' | head -1 | cut -d'"' -f4) && \
	  local_sha=$$(cat .xx-hocon-version) && \
	  if [ "$$remote_sha" = "$$local_sha" ]; then \
	    echo "Expected JSON up to date ($$local_sha)"; exit 0; \
	  fi; \
	fi; \
	tmpdir="$$(mktemp -d)" && \
	trap 'rm -rf "$$tmpdir"' EXIT INT TERM && \
	rm -rf "$(EXPECTED_DIR)" && \
	mkdir -p "$(EXPECTED_DIR)" && \
	curl -sfL "https://github.com/$(TESTDATA_REPO)/archive/$(TESTDATA_REF).tar.gz" -o "$$tmpdir/archive.tar.gz" && \
	tar xzf "$$tmpdir/archive.tar.gz" -C "$$tmpdir" --strip-components=1 && \
	cp -R "$$tmpdir/expected/hocon/." "$(EXPECTED_DIR)/" && \
	if [ -d "$$tmpdir/testdata/hocon/subst-tokenize" ]; then \
	  mkdir -p testdata/hocon/subst-tokenize && \
	  cp "$$tmpdir/testdata/hocon/subst-tokenize/"*.conf testdata/hocon/subst-tokenize/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/env-var-list" ]; then \
	  mkdir -p testdata/hocon/env-var-list && \
	  cp "$$tmpdir/testdata/hocon/env-var-list/"* testdata/hocon/env-var-list/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/concat-errors" ]; then \
	  mkdir -p testdata/hocon/concat-errors && \
	  cp "$$tmpdir/testdata/hocon/concat-errors/"*.conf testdata/hocon/concat-errors/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/units-default" ]; then \
	  mkdir -p testdata/hocon/units-default && \
	  cp "$$tmpdir/testdata/hocon/units-default/"*.conf testdata/hocon/units-default/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/include-reservation" ]; then \
	  mkdir -p testdata/hocon/include-reservation && \
	  cp "$$tmpdir/testdata/hocon/include-reservation/"*.conf testdata/hocon/include-reservation/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/empty-file" ]; then \
	  mkdir -p testdata/hocon/empty-file && \
	  cp "$$tmpdir/testdata/hocon/empty-file/"* testdata/hocon/empty-file/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/byte-single-letter" ]; then \
	  mkdir -p testdata/hocon/byte-single-letter && \
	  cp "$$tmpdir/testdata/hocon/byte-single-letter/"*.conf testdata/hocon/byte-single-letter/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/properties-conflict" ]; then \
	  mkdir -p testdata/hocon/properties-conflict && \
	  cp "$$tmpdir/testdata/hocon/properties-conflict/"* testdata/hocon/properties-conflict/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/unquoted-starts" ]; then \
	  mkdir -p testdata/hocon/unquoted-starts && \
	  cp "$$tmpdir/testdata/hocon/unquoted-starts/"*.conf testdata/hocon/unquoted-starts/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/unquoted-parens" ]; then \
	  mkdir -p testdata/hocon/unquoted-parens && \
	  cp "$$tmpdir/testdata/hocon/unquoted-parens/"*.conf testdata/hocon/unquoted-parens/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/deferred-resolution" ]; then \
	  mkdir -p testdata/hocon/deferred-resolution && \
	  cp "$$tmpdir/testdata/hocon/deferred-resolution/"*.yaml testdata/hocon/deferred-resolution/ 2>/dev/null || true; \
	  cp "$$tmpdir/testdata/hocon/deferred-resolution/README.md" testdata/hocon/deferred-resolution/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/key-hyphen-position" ]; then \
	  mkdir -p testdata/hocon/key-hyphen-position && \
	  cp "$$tmpdir/testdata/hocon/key-hyphen-position/"*.conf testdata/hocon/key-hyphen-position/ 2>/dev/null || true; \
	fi && \
	if [ -d "$$tmpdir/testdata/hocon/path-expr-whitespace" ]; then \
	  mkdir -p testdata/hocon/path-expr-whitespace && \
	  cp "$$tmpdir/testdata/hocon/path-expr-whitespace/"*.conf testdata/hocon/path-expr-whitespace/ 2>/dev/null || true; \
	fi && \
	curl -sf "https://api.github.com/repos/$(TESTDATA_REPO)/commits/$(TESTDATA_REF)" | grep '"sha"' | head -1 | cut -d'"' -f4 > .xx-hocon-version && \
	echo "Done. Fetched $$(cat .xx-hocon-version)"

test:
	go test ./... -count=1
