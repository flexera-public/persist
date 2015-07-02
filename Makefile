#! /usr/bin/make
#
# Makefile for Golang projects, v1 (simplified for libraries)
#
# Features:
# - runs ginkgo tests recursively, computes code coverage report
# - code coverage ready for travis-ci to upload and produce badges for README.md
# - to include the build status and code coverage badge in CI use (replace NAME by what
#   you set $(NAME) to further down, and also replace magnum.travis-ci.com by travis-ci.org for
#   publicly accessible repos [sigh]):
#   [![Build Status](https://magnum.travis-ci.com/rightscale/NAME.svg?token=4Q13wQTY4zqXgU7Edw3B&branch=master)](https://magnum.travis-ci.com/rightscale/NAME
#   ![Code Coverage](https://s3.amazonaws.com/rs-code-coverage/NAME/cc_badge_master.svg)
#
# Top-level targets:
# default: compile the program, you can thus use make && ./NAME -options ...
# test: runs unit tests recursively and produces code coverage stats and shows them
# travis-test: just runs unit tests recursively
#
# ** GOPATH and import dependencies **
# - to compile or run ginkgo like in the Makefile:
#   export GOPATH=`pwd`/.vendor; export PATH="`pwd`/.vendor/bin:$PATH"

#NAME=$(shell basename $$PWD)
NAME=persist
ACL=public-read
# dependencies that are used by the build&test process
DEPEND=golang.org/x/tools/cmd/cover github.com/onsi/ginkgo/ginkgo \
       github.com/onsi/gomega github.com/rlmcpherson/s3gof3r/gof3r \
       github.com/dkulchenko/bunch

#=== below this line ideally remains unchanged, add new targets at the end  ===

TRAVIS_BRANCH?=dev
DATE=$(shell date '+%F %T')
TRAVIS_COMMIT?=$(shell git symbolic-ref HEAD | cut -d"/" -f 3)
# we manually adjust the GOPATH instead of trying to prefix everything with `bunch go`
ifeq ($(OS),Windows_NT)
	SHELL:=/bin/dash
	GOPATH:=$(shell cygpath --windows $(PWD))/.vendor;$(GOPATH)
else
	GOPATH:=$(PWD)/.vendor:$(GOPATH)
endif
# we build $(DEPEND) binaries into the .vendor subdir
PATH:=$(PWD)/.vendor/bin:$(PATH)

.PHONY: depend clean default

# the default target builds a binary in the top-level dir for whatever the local OS is
default: $(NAME)
$(NAME): *.go version
	go build -o $(NAME) .

gopath:
	@echo export GOPATH="$(GOPATH)"

# gofmt uses the awkward *.go */*.go because gofmt -l . descends into the .vendor tree
# and then pointlessly complains about bad formatting in imported packages, sigh
lint:
	@if gofmt -l *.go | grep .go; then \
	  echo "^- Repo contains improperly formatted go files; run gofmt -w *.go" && exit 1; \
	  else echo "All .go files formatted correctly"; fi
	go tool vet -composites=false *.go
#	go tool vet -composites=false **/*.go

travis-test: cover

# running ginkgo twice, sadly, the problem is that -cover modifies the source code with the effect
# that if there are errors the output of gingko refers to incorrect line numbers
# tip: if you don't like colors use gingkgo -r -noColor
test: lint gopath
	ginkgo -r --randomizeAllSpecs --randomizeSuites --failOnPending

race: lint
	ginkgo -r --randomizeAllSpecs --randomizeSuites --failOnPending --race

cover: lint
	ginkgo -r --randomizeAllSpecs --randomizeSuites --failOnPending -cover
	@echo 'mode: atomic' >_total
	@for f in `find . -name \*.coverprofile`; do tail -n +2 $$f >>_total; done
	@mv _total total.coverprofile
	@COVERAGE=$$(go tool cover -func=total.coverprofile | grep "^total:" | grep -o "[0-9\.]*");\
	  echo "*** Code Coverage is $$COVERAGE% ***"
	@echo Details: go tool cover -func=total.coverprofile
