# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
BIN_PATH = $(shell pwd)/bin
export PATH := $(shell pwd)/bin:$(PATH)

CONTROLLERS=controllers job-task-runner kpack-image-builder statefulset-runner
COMPONENTS=api $(CONTROLLERS)

manifests: install-controller-gen
	$(CONTROLLER_GEN) \
		paths="./model/..." \
		crd \
		output:crd:artifacts:config=helm/korifi/controllers/crds
	@for comp in $(COMPONENTS); do make -C $$comp manifests; done

generate: install-controller-gen
	$(CONTROLLER_GEN) object:headerFile="controllers/hack/boilerplate.go.txt" paths="./model/..."
	@for comp in $(CONTROLLERS); do make -C $$comp generate; done
	go run ./scripts/helmdoc/main.go > README.helm.md

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
install-controller-gen:
	GOBIN=$(shell pwd)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen

generate-fakes:
	go generate ./...

fmt: install-gofumpt install-shfmt
	$(GOFUMPT) -w .
	$(SHFMT) -f . | grep -v '^tests/vendor' | xargs $(SHFMT) -w -i 2 -ci

vet: ## Run go vet against code.
	go vet ./...

lint: fmt vet gosec staticcheck golangci-lint

gosec: install-gosec
	$(GOSEC) --exclude=G101,G304,G401,G404,G505 --exclude-dir=tests ./...

staticcheck: install-staticcheck
	$(STATICCHECK) ./...

golangci-lint: install-golangci-lint
	$(GOLANGCILINT) run

test: lint
	@for comp in $(COMPONENTS); do make -C $$comp test; done
	make test-tools
	make test-e2e

test-tools:
	./scripts/run-tests.sh tools

test-e2e: build-dorifi
	./scripts/run-tests.sh tests/e2e

test-crds: build-dorifi
	./scripts/run-tests.sh tests/crds

test-smoke: build-dorifi bin/cf
	./scripts/run-tests.sh tests/smoke


build-dorifi:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -C tests/assets/dorifi-golang -o ../dorifi/dorifi .
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -C tests/assets/dorifi-golang -o ../multi-process/dorifi .
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -C tests/assets/sample-broker-golang -o ../sample-broker/sample-broker .

GOFUMPT = $(shell go env GOPATH)/bin/gofumpt
install-gofumpt:
	go install mvdan.cc/gofumpt@latest

SHFMT = $(shell go env GOPATH)/bin/shfmt
install-shfmt:
	go install mvdan.cc/sh/v3/cmd/shfmt@latest

VENDIR = $(shell go env GOPATH)/bin/vendir
install-vendir:
	go install carvel.dev/vendir/cmd/vendir@latest

GOSEC = $(shell go env GOPATH)/bin/gosec
install-gosec:
	go install github.com/securego/gosec/v2/cmd/gosec@latest

STATICCHECK = $(shell go env GOPATH)/bin/staticcheck
install-staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@latest

GOLANGCILINT = $(shell go env GOPATH)/bin/golangci-lint
install-golangci-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

bin/cf:
	mkdir -p $(BIN_PATH)
	curl -fsSL "https://packages.cloudfoundry.org/stable?release=linux64-binary&version=v8&source=github" \
	  | tar -zx cf8 \
	  && mv cf8 $(BIN_PATH)/cf \
	  && chmod +x $(BIN_PATH)/cf


vendir-update-dependencies: install-vendir
	$(VENDIR) sync --chdir tests
