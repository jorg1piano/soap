binary := "soap"

# build the binary
build:
    go build -o {{binary}} .

# build and run TUI
run: build
    ./{{binary}}

# install soap to ~/.soap/
install: build
    mkdir -p ~/.soap
    cp {{binary}} ~/.soap/soap
    codesign --force --deep --sign - ~/.soap/soap
    xattr -d com.apple.provenance ~/.soap/soap 2>/dev/null || true
    rm {{binary}}
    @echo "Installed to ~/.soap/soap (signed and quarantine removed)"
    @echo 'Add ~/.soap to your PATH: export PATH="$HOME/.soap:$PATH"'

# remove build artifacts
clean:
    rm -f {{binary}}

# kill processes, wipe NATS data, clean worktrees
nuke:
    pkill -f 'soap server' || true
    rm -rf /tmp/soap /tmp/soap.port
    rm -rf .worktrees/
    @echo "Cleaned up server data and worktrees"

# reset state then start fresh
fresh: nuke build run

# run go vet
vet:
    go vet ./...
