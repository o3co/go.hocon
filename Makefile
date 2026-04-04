TESTDATA_REPO  := o3co/xx.hocon
TESTDATA_REF   := main
EXPECTED_DIR   := testdata/expected

.PHONY: testdata test

testdata:
	@echo "Fetching expected JSON from $(TESTDATA_REPO)@$(TESTDATA_REF)..."
	@tmpdir="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmpdir"' EXIT INT TERM; \
	mkdir -p "$(EXPECTED_DIR)"; \
	curl -sL "https://github.com/$(TESTDATA_REPO)/archive/$(TESTDATA_REF).tar.gz" -o "$$tmpdir/archive.tar.gz"; \
	tar xzf "$$tmpdir/archive.tar.gz" -C "$$tmpdir" --strip-components=1; \
	cp -R "$$tmpdir/expected/hocon/." "$(EXPECTED_DIR)/"; \
	echo "Done."

test:
	go test ./... -count=1
