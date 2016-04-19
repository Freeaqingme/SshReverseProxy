export GOPATH:=$(shell pwd)

GO        ?= go
PKG       := ./src/sshReverseProxy/
BUILDTAGS := debug
VERSION   ?= $(shell git describe --dirty --tags | sed 's/^v//' )

.PHONY: default
default: all

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
	go install -tags '$(BUILDTAGS)' -ldflags '$(LDFLAGS)' sshReverseProxy

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

.PHONY: deb
deb: release
	rm -rf pkg_root/
	mkdir -p pkg_root/lib/systemd/system/
	cp dist/sshReverseProxy.service pkg_root/lib/systemd/system/sshreverseproxy.service
	mkdir -p pkg_root/etc/default
	cp dist/debian/defaults pkg_root/etc/default/sshreverseproxy
	mkdir -p pkg_root/usr/bin/
	cp bin/sshReverseProxy pkg_root/usr/bin/sshreverseproxy
	mkdir -p pkg_root/usr/share/doc/sshreverseproxy
	cp LICENSE pkg_root/usr/share/doc/sshreverseproxy/
	mkdir -p pkg_root/etc/sshreverseproxy
	cp sshReverseProxy.conf.dist pkg_root/etc/sshreverseproxy/sshreverseproxy.conf
	mkdir -p pkg_root/etc/logrotate.d
	cp dist/debian/logrotate pkg_root/etc/logrotate.d/sshreverseproxy
	fpm \
		-n sshreverseproxy \
		-C pkg_root \
		-s dir \
		-t deb \
		-v $(VERSION) \
		--force \
		--deb-compression bzip2 \
		--after-install dist/debian/postinst \
		--before-remove dist/debian/prerm \
		--license BSD-2-clause \
		-m "Dolf Schimmel <dolf@transip.nl>" \
		--url "https://github.com/Freeaqingme/sshReverseProxy" \
		--vendor "github.com/Freeaqingme" \
		--description "A layer-7 reverse proxy for the SSH/SFTP protocol." \
		--category network \
		--config-files /etc/sshreverseproxy/sshreverseproxy.conf \
		--directories /var/run/sshreverseproxy \
		.
