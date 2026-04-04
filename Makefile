TESTDATA_REPO  := o3co/xx.hocon
TESTDATA_REF   := main
EXPECTED_DIR   := testdata/expected

.PHONY: testdata test

testdata:
	@echo "Fetching expected JSON from $(TESTDATA_REPO)@$(TESTDATA_REF)..."
	@rm -rf /tmp/xx-hocon-dl
	@mkdir -p /tmp/xx-hocon-dl $(EXPECTED_DIR)
	@gh api repos/$(TESTDATA_REPO)/tarball/$(TESTDATA_REF) > /tmp/xx-hocon-dl/archive.tar.gz
	@tar xzf /tmp/xx-hocon-dl/archive.tar.gz -C /tmp/xx-hocon-dl --strip-components=1
	@rsync -a /tmp/xx-hocon-dl/expected/hocon/ $(EXPECTED_DIR)/
	@rm -rf /tmp/xx-hocon-dl
	@echo "Done."

test: testdata
	go test ./... -count=1
