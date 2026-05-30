BINARY  = tfr
LDFLAGS = -ldflags="-s -w"
GOTOOLCHAIN_ENV = GOTOOLCHAIN=local

.PHONY: all linux windows darwin arm clean

all: linux

linux:
	$(GOTOOLCHAIN_ENV) GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)_linux_amd64 .

windows:
	$(GOTOOLCHAIN_ENV) GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)_windows_amd64.exe .

darwin:
	$(GOTOOLCHAIN_ENV) GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)_darwin_amd64 .

arm:
	$(GOTOOLCHAIN_ENV) GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)_linux_arm64 .

clean:
	rm -f $(BINARY) $(BINARY)_* 
