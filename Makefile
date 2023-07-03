MKFILE_PATH := $(lastword $(MAKEFILE_LIST))
PARENT_DIR := $(abspath $(patsubst %/,%,$(dir $(abspath $(MKFILE_PATH)))))

# Tags specific for building
GOTAGS ?=

# Number of procs to use
GOMAXPROCS ?= 4

# Common project props
GOVERSION ?= 1.20
OWNER ?= australia-southeast1-docker.pkg.dev/field-engineering-apac/public-repo
GIT_COMMIT ?= $(shell git rev-parse --short HEAD)

# Get the main project props
NAME ?= recaptcha-processor
PROJ_CTX_DIR = processing-server
PROJ_DIR = ${PARENT_DIR}/${PROJ_CTX_DIR}
VERSION ?= $(shell git describe --tags --always --dirty 2> /dev/null || echo v0)
LD_FLAGS ?= -s \
	-w \
	-X 'github.com/pseudonator/recaptcha-processing-server/pkg/version.Name=${NAME}' \
	-X 'github.com/pseudonator/recaptcha-processing-server/pkg/version.Version=${VERSION}' \
	-X 'github.com/pseudonator/recaptcha-processing-server/pkg/version.GitCommit=${GIT_COMMIT}'

# Test services props
TEST_PROJ_DIR = ${PARENT_DIR}/test
TEST_TARGETS ?= client backend-server
TEST_BIN_PREFIX_NAME ?= recaptcha-processor-test
TEST_LD_FLAGS ?= -s -w

# Current system information
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Default os-arch combination to build
XC_OS ?= darwin linux windows
XC_ARCH ?= amd64 arm64
XC_EXCLUDE ?=

DOCKER_SUPPORTED_PLATFORMS ?= linux/amd64,linux/arm64

# Output dir for binaries
BIN_OUT_DIR ?= bin

BUILD_OPTS ?=
ifeq ($(PUSH_MULTIARCH), true)
BUILDX_ARG_PUSH = '--push'
endif

## all: run all targets
all: clean test tidy build
.PHONY: all

## build all the components
build: build-test-services build-processor
.PHONY: build

## docker: build and push all the images (use PUSH_MULTIARCH=true)
docker: docker-build-test-services docker-build-processor
.PHONE: docker

# ------------------------------------------------------------------------------------------------------------
# Targets for test services (services for testing recaptcha-processor)
# ------------------------------------------------------------------------------------------------------------

# macro for building all the test targets
define build-xc-test-services
  $(BIN_OUT_DIR)/$1/$2_$3/$(TEST_BIN_PREFIX_NAME)-$1$(if $(findstring windows,$2),.exe,):
  ifneq (,$(findstring ${2}/${3},$(XC_EXCLUDE)))
		@printf "==> Building %s%20s %s\n" "-->" "${2}/${3}:" "${TEST_PROJ_DIR}/${1} (excluded)"
  else
		@printf "==> Building %s%20s %s\n" "-->" "${2}/${3}:" "${TEST_PROJ_DIR}/${1}"
		@docker run \
			--interactive \
			--rm \
			--dns="8.8.8.8" \
			--volume="${PARENT_DIR}/bin:/go/src/bin" \
			--volume="${TEST_PROJ_DIR}/${1}:/go/src/build" \
			--workdir="/go/src/build" \
			"golang:${GOVERSION}" \
			env \
				CGO_ENABLED="0" \
				GOOS="${2}" \
				GOARCH="${3}" \
				go build \
					-a \
					-o="/go/src/bin/${1}/${2}_${3}/${TEST_BIN_PREFIX_NAME}-${1}${4}" \
					-ldflags "${TEST_LD_FLAGS}" \
					-tags "${GOTAGS}" \
					./cmd/server/main.go
  endif

  ## build-<test-target>: for building a single test target (test-target is either 'client' or 'backend-server')
  build-$(1):: $(BIN_OUT_DIR)/$1/$2_$3/$(TEST_BIN_PREFIX_NAME)-$1$(if $(findstring windows,$2),.exe,)
  .PHONY: build-$(1)

  ## build-<test-target>-<os>: building a single os test target (test-target is either 'client' or 'backend-server')
  build-$(1)-$(2):: $(BIN_OUT_DIR)/$1/$2_$3/$(TEST_BIN_PREFIX_NAME)-$1$(if $(findstring windows,$2),.exe,)
  .PHONY: build-$(1)-$(2)

  ## build-test-services: building all test targets
  build-test-services:: $(BIN_OUT_DIR)/$1/$2_$3/$(TEST_BIN_PREFIX_NAME)-$1$(if $(findstring windows,$2),.exe,)
endef
$(foreach prj,$(TEST_TARGETS),$(foreach goarch,$(XC_ARCH),$(foreach goos,$(XC_OS),$(eval $(call build-xc-test-services,$(prj),$(goos),$(goarch),$(if $(findstring windows,$(goos)),.exe,))))))

# multiarch docker build and push
define build-docker-xc-test-services
  docker-build-$(1): build-test-services _prepare-multiarch
		@echo "==> Building Docker multi-arch images for $1"
		@docker buildx build \
			--rm \
			--force-rm \
			--no-cache \
			--compress \
			--file="${TEST_PROJ_DIR}/${1}/Dockerfile" \
			--platform ${DOCKER_SUPPORTED_PLATFORMS} \
			--build-arg="NAME=${1}" \
			--build-arg="NAME_PREFIX=${TEST_BIN_PREFIX_NAME}" \
			--tag="${OWNER}/${TEST_BIN_PREFIX_NAME}-${1}" \
			--tag="${OWNER}/${TEST_BIN_PREFIX_NAME}-${1}:${VERSION}" \
			$(BUILDX_ARG_PUSH) \
			"${PARENT_DIR}"
  .PHONY: docker-build-$(1)

  ## docker-build-test-services: building and pushing all test target images (If you need to publish use PUSH_MULTIARCH=true)
  docker-build-test-services:: docker-build-$(1)
endef
$(foreach prj,$(TEST_TARGETS),$(eval $(call build-docker-xc-test-services,$(prj),)))

# ------------------------------------------------------------------------------------------------------------

# ------------------------------------------------------------------------------------------------------------
# Targets for recaptcha-processor
# ------------------------------------------------------------------------------------------------------------

# macro for building recaptcha-processor
define make-xc-target
  $(BIN_OUT_DIR)/$(NAME)/$1_$2/$(NAME)$(if $(findstring windows,$1),.exe,):
  ifneq (,$(findstring ${1}/${2},$(XC_EXCLUDE)))
		@printf "==> Building %s%20s %s\n" "-->" "${1}/${2}:" "${PROJ_DIR} (excluded)"
  else
		@printf "==> Building %s%20s %s\n" "-->" "${1}/${2}:" "${PROJ_DIR}"
		@docker run \
			--interactive \
			--rm \
			--dns="8.8.8.8" \
			--volume="${PARENT_DIR}/${BIN_OUT_DIR}:/go/src/bin" \
			--volume="${PROJ_DIR}:/go/src/build" \
			--workdir="/go/src/build" \
			"golang:${GOVERSION}" \
			env \
				CGO_ENABLED="0" \
				GOOS="${1}" \
				GOARCH="${2}" \
				go build \
					-a \
					-o="/go/src/bin/${NAME}/${1}_${2}/${NAME}${3}" \
					-ldflags "${LD_FLAGS}" \
					-tags "${GOTAGS}" \
					./cmd/server/main.go
  endif

  ## build-processor-<os>: os target for building binary executable for recaptcha-processor
  build-processor-$(1):: $(BIN_OUT_DIR)/$(NAME)/$1_$2/$(NAME)$(if $(findstring windows,$1),.exe,)
  .PHONY: build-processor-$(1)

  ## build-processor: target for building all os binary executables for recaptcha-processor
  build-processor:: $(BIN_OUT_DIR)/$(NAME)/$1_$2/$(NAME)$(if $(findstring windows,$1),.exe,)
endef
$(foreach goarch,$(XC_ARCH),$(foreach goos,$(XC_OS),$(eval $(call make-xc-target,$(goos),$(goarch),$(if $(findstring windows,$(goos)),.exe,)))))

## docker-build-processor: building and pushing multiarch docker image for recaptcha-processor (If you need to publish use PUSH_MULTIARCH=true)
docker-build-processor: build-processor _prepare-multiarch
	@echo "==> Building Docker multi-arch images for recaptcha-processor"
	@docker buildx build \
		--rm \
		--force-rm \
		--no-cache \
		--compress \
		--file="${PROJ_CTX_DIR}/Dockerfile" \
		--platform ${DOCKER_SUPPORTED_PLATFORMS} \
		--build-arg="NAME=${NAME}" \
		--tag="${OWNER}/${NAME}" \
		--tag="${OWNER}/${NAME}:${VERSION}" \
		$(BUILDX_ARG_PUSH) \
		"${PARENT_DIR}"
.PHONY: docker-build-processor

## tidy: format the source
tidy:
	@pushd ${PROJ_CTX_DIR} >/dev/null;go fmt ./...;go mod tidy -v;popd >/dev/null
.PHONY: tidy

## test: run the test suite
test:
	@pushd ${PROJ_CTX_DIR} >/dev/null;go test -v -race -buildvcs ./...;popd >/dev/null
.PHONY: test

# ------------------------------------------------------------------------------------------------------------

clean:
	rm -rf bin
.PHONY: clean

# ------------------------------------------------------------------------------------------------------------
# Helper rules #
# ------------------------------------------------------------------------------------------------------------
# for starting the buildx container
_prepare-multiarch:
	@docker buildx inspect | grep 'Driver:' | grep 'docker-container' > /dev/null || { docker buildx create --use --name "${NAME}-builder"; docker buildx inspect --bootstrap; }

# help generator
help:
	@echo 'Usage:'
	@sed -n 's/^[ \t]*##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /' | sort
.PHONY: help