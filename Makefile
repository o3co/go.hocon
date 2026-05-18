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
	if [ -d "$$tmpdir/testdata/hocon/include-reservation" ]; then \
	  mkdir -p testdata/hocon/include-reservation && \
	  cp "$$tmpdir/testdata/hocon/include-reservation/"*.conf testdata/hocon/include-reservation/ 2>/dev/null || true; \
	fi && \
	curl -sf "https://api.github.com/repos/$(TESTDATA_REPO)/commits/$(TESTDATA_REF)" | grep '"sha"' | head -1 | cut -d'"' -f4 > .xx-hocon-version && \
	echo "Done. Fetched $$(cat .xx-hocon-version)"

test:
	go test ./... -count=1
