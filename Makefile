export GOPATH:=$(shell pwd)

GO        ?= go
PKG       := ./src/sshReverseProxy/
BUILDTAGS := debug
VERSION   ?= $(shell git describe --dirty --tags | sed 's/^v//' )

THIS_FILE := $(lastword $(MAKEFILE_LIST))
export PATH   := ./bin:$(PATH)
export GOOS   ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
export GOARCH ?= amd64


.PHONY: default
default: all

# find src/ -name .git -type d | sed -s 's/.git$//' | while read line; do echo -n "${line} " | sed 's/^src\///'; git -C $line rev-parse HEAD; done | sort > GLOCKFILE
.PHONY: deps
deps:
	go get -tags '$(BUILDTAGS)' -d -v sshReverseProxy/...
	go get github.com/robfig/glock
	git diff /dev/null GLOCKFILE | ./bin/glock apply .

.PHONY: sshReverseProxy
sshReverseProxy: deps binary

.PHONY: binary
binary: LDFLAGS += -X "main.buildTag=v$(VERSION)"
binary: LDFLAGS += -X "main.buildTime=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')"
binary:
	go build -o bin/sshReverseProxy-$(GOOS)-$(GOARCH) -tags '$(BUILDTAGS)' -ldflags '$(LDFLAGS)' sshReverseProxy

.PHONY: release
release: BUILDTAGS=release
release: sshReverseProxy

.PHONY: fmt
fmt:
	go fmt sshReverseProxy/...

.PHONY: all
all: fmt sshReverseProxy

.PHONY: clean
clean:
	rm -rf bin/
	rm -rf pkg/
	rm -rf src/sshReverseProxy/assets/
	go clean -i -r sshReverseProxy

.PHONY: pkg
pkg:
	GOOS=linux $(MAKE) -f $(THIS_FILE) pkg_archive
	GOOS=freebsd $(MAKE) -f $(THIS_FILE) pkg_archive

.PHONY: pkg_root
pkg_root: release
	rm -rf pkg_root/
        ifeq ($(GOOS),linux)
		mkdir -p pkg_root/deb/lib/systemd/system/
		cp dist/sshReverseProxy.service pkg_root/deb/lib/systemd/system/sshreverseproxy.service
		mkdir -p pkg_root/deb/etc/default
		cp dist/debian/defaults pkg_root/deb/etc/default/sshreverseproxy
		mkdir -p pkg_root/deb/usr/bin/
		cp bin/sshReverseProxy-$(GOOS)-$(GOARCH) pkg_root/deb/usr/bin/sshreverseproxy
		mkdir -p pkg_root/deb/usr/share/doc/sshreverseproxy
		cp LICENSE pkg_root/deb/usr/share/doc/sshreverseproxy/
		mkdir -p pkg_root/deb/etc/sshreverseproxy
		cp sshReverseProxy.conf.dist pkg_root/deb/etc/sshreverseproxy/sshreverseproxy.conf
		mkdir -p pkg_root/deb/etc/logrotate.d
		cp dist/debian/logrotate pkg_root/deb/etc/logrotate.d/sshreverseproxy
        else ifeq ($(GOOS),freebsd)
		mkdir -p pkg_root/freebsd/usr/local/etc/rc.conf.d/
		cp dist/freebsd/rc.conf pkg_root/freebsd/usr/local/etc/rc.conf.d/sshreverseproxy
		mkdir -p pkg_root/freebsd/usr/local/etc/rc.d/
		cp dist/freebsd/rc.sh pkg_root/freebsd/usr/local/etc/rc.d/sshreverseproxy
		chmod +x pkg_root/freebsd/usr/local/etc/rc.d/sshreverseproxy
		mkdir -p pkg_root/freebsd/usr/bin/
		mv ./bin/sshReverseProxy-$(GOOS)-$(GOARCH) pkg_root/freebsd/usr/bin/sshreverseproxy
		chmod +x pkg_root/freebsd/usr/bin/sshreverseproxy
		mkdir -p pkg_root/freebsd/usr/local/etc/sshreverseproxy
		cp sshReverseProxy.conf.dist pkg_root/freebsd/usr/local/etc/sshreverseproxy/sshreverseproxy.conf
        endif

# Requires this patch: https://github.com/jordansissel/fpm/pull/1140/files
.PHONY: pkg_archive
pkg_archive: pkg_root
        ifeq ($(GOOS),linux)
		$(eval type=deb)
		$(eval distdir=debian)
		$(eval conffile=/etc/sshreverseproxy/sshreverseproxy.conf)
        else ifeq ($(GOOS),freebsd)
		$(eval type=freebsd)
		$(eval distdir=freebsd)
		$(eval conffile=/usr/local/etc/wh-queue-daemon/wh-queue-daemon.conf)
        endif
	fpm \
		-n sshreverseproxy \
		-C pkg_root/$(type) \
		-s dir \
		-t $(type) \
		-v "$(VERSION)" \
		--force \
		--freebsd-origin freebsd:10:x86:64 \
		--deb-compression bzip2 \
		--after-install dist/$(distdir)/postinst \
		--before-remove dist/$(distdir)/prerm \
		--license BSD-2-clause \
		-m "Dolf Schimmel <dolf@transip.nl>" \
		--url "https://github.com/Freeaqingme/sshReverseProxy" \
		--vendor "github.com/Freeaqingme" \
		--description "A layer-7 reverse proxy for the SSH/SFTP protocol." \
		--category network \
		--config-files /etc/sshreverseproxy/sshreverseproxy.conf \
		--directories /var/run/sshreverseproxy \
		.
