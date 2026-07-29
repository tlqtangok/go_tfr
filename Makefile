BINARY  = tfr
LDFLAGS = -ldflags="-s -w"
GOTOOLCHAIN_ENV = GOTOOLCHAIN=local CGO_ENABLED=0

.PHONY: all linux windows darwin darwin-arm arm clean

all: linux windows darwin darwin-arm arm

linux:
	$(GOTOOLCHAIN_ENV) GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)_linux_amd64 .

windows:
	$(GOTOOLCHAIN_ENV) GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)_windows_amd64.exe .

darwin:
	$(GOTOOLCHAIN_ENV) GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)_darwin_amd64 .

darwin-arm:
	$(GOTOOLCHAIN_ENV) GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)_darwin_arm64 .

arm:
	$(GOTOOLCHAIN_ENV) GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)_linux_arm64 .

clean:
	rm -f $(BINARY) $(BINARY)_* 
