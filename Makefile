all: depend build test

SHELL := /bin/bash
.DEFAULT_GOAL := all
BACKUP=gpbackup
RESTORE=gprestore
DIR_PATH=$(shell dirname `pwd`)
BIN_DIR=$(HOME)/go/bin

GIT_VERSION := $(shell git describe --tags | awk -F "-" '{$$2+=0; print $$1 "." $$2}')
DEV_VERSION := $(shell git diff | wc -l | awk '{if($$1!=0) {print "_dev"}}')
BACKUP_VERSION_STR="-X github.com/greenplum-db/gpbackup/backup.version=$(GIT_VERSION)$(DEV_VERSION)"
RESTORE_VERSION_STR="-X github.com/greenplum-db/gpbackup/restore.version=$(GIT_VERSION)$(DEV_VERSION)"

DEST = .

GOFLAGS :=
dependencies :
		go get github.com/jmoiron/sqlx
		go get github.com/lib/pq
		go get github.com/maxbrunsfeld/counterfeiter
		go get github.com/onsi/ginkgo/ginkgo
		go get github.com/onsi/gomega
		go get github.com/pkg/errors
		go get golang.org/x/tools/cmd/goimports
		go get gopkg.in/DATA-DOG/go-sqlmock.v1
		go get -u github.com/golang/lint/golint
		go get github.com/alecthomas/gometalinter

format :
		goimports -w .
		gofmt -w -s .

lint :
		! gofmt -l . | read
		gometalinter --config=gometalinter.config ./...

unit :
		ginkgo -r -randomizeSuites -randomizeAllSpecs backup restore utils testutils 2>&1

integration :
		ginkgo -r -randomizeSuites -randomizeAllSpecs integration 2>&1

test : lint unit integration

depend : dependencies

build :
		go build -tags '$(BACKUP)' $(GOFLAGS) -o $(BIN_DIR)/$(BACKUP) -ldflags $(BACKUP_VERSION_STR)
		go build -tags '$(RESTORE)' $(GOFLAGS) -o $(BIN_DIR)/$(RESTORE) -ldflags $(RESTORE_VERSION_STR)

build_linux :
		env GOOS=linux GOARCH=amd64 go build -tags '$(BACKUP)' $(GOFLAGS) -o $(BIN_DIR)/$(BACKUP) -ldflags $(BACKUP_VERSION_STR)
		env GOOS=linux GOARCH=amd64 go build -tags '$(RESTORE)' $(GOFLAGS) -o $(BIN_DIR)/$(RESTORE) -ldflags $(RESTORE_VERSION_STR)

build_mac :
		env GOOS=darwin GOARCH=amd64 go build -tags '$(BACKUP)' $(GOFLAGS) -o $(BIN_DIR)/$(BACKUP) -ldflags $(BACKUP_VERSION_STR)
		env GOOS=darwin GOARCH=amd64 go build -tags '$(RESTORE)' $(GOFLAGS) -o $(BIN_DIR)/$(RESTORE) -ldflags $(RESTORE_VERSION_STR)

install : all installdirs
		$(INSTALL_PROGRAM) gpbackup$(X) '$(DESTDIR)$(bindir)/gpbackup$(X)'

installdirs :
		$(MKDIR_P) '$(DESTDIR)$(bindir)'

clean :
		rm -f $(BIN_DIR)/$(BACKUP)
		rm -f $(BIN_DIR)/$(RESTORE)
		rm -rf /tmp/go-build*
		rm -rf /tmp/ginkgo*

update_pipeline :
	fly -t gpdb set-pipeline -p gpbackup -c ci/pipeline.yml -l <(lpass show "Concourse Credentials" --notes)

push : format
	git pull -r && make test && git push

.PHONY : update_pipeline integration
