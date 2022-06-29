TEST_COUNT ?= 1
TEST ?= go test -count=$(TEST_COUNT) $(TEST_FLAGS) $(if $(V),-v,)

.PHONY: dockersource
dockersource:
	CGO_ENABLED=0 go build

clean:
	rm dockersource

.PHONY: e2e
e2e: dockersource
	$(TEST) ./test
