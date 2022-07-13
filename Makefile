TEST_COUNT ?= 1
TEST ?= go test -count=$(TEST_COUNT) $(TEST_FLAGS) $(if $(V),-v,)

.PHONY: gnarly
gnarly:
	CGO_ENABLED=0 go build $(if $(OUTPUT),-o $(OUTPUT)/$(@),) .

clean:
	rm gnarly

.PHONY: e2e
e2e: gnarly
	$(TEST) ./test
