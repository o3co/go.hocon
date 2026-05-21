// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

// TestRenderJSON exposes the private renderJSON method for use by external
// test files (deferred_resolution_test.go, deferred_resolution_fixture_test.go).
// Lives in *_test.go so it is not compiled into the public binary.
func RenderJSON_ForTest(c *Config) (string, error) {
	return c.renderJSON()
}
