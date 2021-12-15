SHELL:=/bin/bash

.PHONY: test

test:
	go test -parallel 1
