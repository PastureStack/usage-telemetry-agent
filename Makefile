# Modified by PastureStack in 2026: exact-toolchain build and release gates.
.PHONY: all build test validate integration-test package verify-artifact clean

all: validate test build

build:
	./scripts/build

test:
	./scripts/test

validate:
	./scripts/validate

integration-test: build
	./scripts/integration-test

package: build
	./scripts/package

verify-artifact: package
	./scripts/verify-artifact

clean:
	rm -rf bin dist
