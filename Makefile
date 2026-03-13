PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
MANDIR ?= $(PREFIX)/share/man/man1
VERSION := $(shell grep '^version' Cargo.toml | head -1 | sed 's/.*"\(.*\)"/\1/')

.PHONY: build release install uninstall test lint clean setup

build:
	cargo build

release:
	cargo build --release

test:
	cargo test

lint:
	cargo clippy -- -W warnings
	cargo fmt -- --check

install: release
	install -d $(BINDIR)
	install -m 755 target/release/yelo $(BINDIR)/yelo
	@if [ "$$(uname)" = "Darwin" ]; then codesign --sign - --force $(BINDIR)/yelo 2>/dev/null || true; fi
	@if [ -d $(MANDIR) ]; then install -m 644 doc/yelo.1 $(MANDIR)/yelo.1; fi
	@echo "yelo $(VERSION) installed to $(BINDIR)/yelo"

uninstall:
	rm -f $(BINDIR)/yelo
	rm -f $(MANDIR)/yelo.1

clean:
	cargo clean

setup:
	git config core.hooksPath hooks
	@echo "git hooks configured"
