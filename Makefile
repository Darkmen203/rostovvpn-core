.ONESHELL:
PRODUCT_NAME=libcore
BASENAME=$(PRODUCT_NAME)
BINDIR=bin
LIBNAME=$(PRODUCT_NAME)
CLINAME=RostovVPNCli
VPNCLI_NAME=rvpncli
WIN_HELPER_NAME=rostovvpn-helper
MAC_HELPER_NAME=rostovvpn-helper
LINUX_RPATH=-Wl,-rpath,\$$ORIGIN/lib -Wl,--enable-new-dtags

BRANCH=$(shell git branch --show-current)
VERSION=$(shell git describe --tags || echo "unknown version")
ifeq ($(OS),Windows_NT)
Not available for Windows! use bash in WSL
endif

TAGS=with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api,with_grpc,with_v2ray,with_reality,bydll
IOS_ADD_TAGS=with_dhcp,with_low_memory,with_conntrack
GOBUILDLIB=CGO_ENABLED=1 go build -trimpath -tags $(TAGS) -ldflags="-w -s" -buildmode=c-shared
GOBUILDSRV=CGO_ENABLED=1 go build -ldflags "-s -w" -trimpath -tags $(TAGS)

.PHONY: protos
protos:
	protoc --go_out=./ --go-grpc_out=./ --proto_path=rostovvpnrpc rostovvpnrpc/*.proto
	protoc --js_out=import_style=commonjs,binary:./extension/html/rpc/ --grpc-web_out=import_style=commonjs,mode=grpcwebtext:./extension/html/rpc/ --proto_path=rostovvpnrpc rostovvpnrpc/*.proto
	npx browserify extension/html/rpc/extension.js >extension/html/rpc.js


lib_install:
	go install -v github.com/sagernet/gomobile/cmd/gomobile@v0.1.8
	go install -v github.com/sagernet/gomobile/cmd/gobind@v0.1.8
	npm install

headers:
	go build -buildmode=c-archive -o $(BINDIR)/$(LIBNAME).h ./custom

android: lib_install
	gomobile bind -v -androidapi=21 -javapkg=io.nekohasekai -libname=box -tags=$(TAGS) -trimpath -target=android -o $(BINDIR)/$(LIBNAME).aar github.com/sagernet/sing-box/experimental/libbox ./mobile

ios-full: lib_install
	gomobile bind -v  -target ios,iossimulator,tvos,tvossimulator,macos -libname=box -tags=$(TAGS),$(IOS_ADD_TAGS) -trimpath -ldflags="-w -s" -o $(BINDIR)/$(PRODUCT_NAME).xcframework github.com/sagernet/sing-box/experimental/libbox ./mobile 
	mv $(BINDIR)/$(PRODUCT_NAME).xcframework $(BINDIR)/$(LIBNAME).xcframework 
	cp Libcore.podspec $(BINDIR)/$(LIBNAME).xcframework/

ios: lib_install
	gomobile bind -v  -target ios -libname=box -tags=$(TAGS),$(IOS_ADD_TAGS) -trimpath -ldflags="-w -s" -o $(BINDIR)/Libcore.xcframework github.com/sagernet/sing-box/experimental/libbox ./mobile
	cp Info.plist $(BINDIR)/Libcore.xcframework/


webui:
	curl -L -o webui.zip  https://github.com/Darkmen203/Yacd-meta/archive/gh-pages.zip 
	unzip -d ./ -q webui.zip
	rm webui.zip
	rm -rf bin/webui
	mv Yacd-meta-gh-pages bin/webui

.PHONY: build
windows-amd64:
	curl http://localhost:18020/exit || echo "exited"
	env GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GOBUILDLIB) -o $(BINDIR)/$(LIBNAME).dll ./custom
	go install -mod=readonly github.com/akavel/rsrc@latest ||echo "rsrc error in installation"
	go run ./cli tunnel exit
	cp $(BINDIR)/$(LIBNAME).dll ./$(LIBNAME).dll 
	# --- важное: пересоздаём syso с манифестом ---
	rm -f ./cli/bydll/cli.syso
	$$(go env GOPATH)/bin/rsrc \
	  -ico ./assets/rostovvpn-cli.ico \
	  -manifest ./cli/bydll/cli.manifest \
	  -o ./cli/bydll/cli.syso || echo "rsrc error in syso"
	# ------------------------------------------------
# 	$$(go env GOPATH)/bin/rsrc -ico ./assets/rostovvpn-cli.ico -o ./cli/bydll/cli.syso ||echo "rsrc error in syso"
	env GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc CGO_LDFLAGS="$(LIBNAME).dll" $(GOBUILDSRV) -o $(BINDIR)/$(CLINAME).exe ./cli/bydll
	rm ./$(LIBNAME).dll
	make webui
	

linux-amd64:
	mkdir -p $(BINDIR)/lib
	env GOOS=linux GOARCH=amd64 $(GOBUILDLIB) -o $(BINDIR)/lib/$(LIBNAME).so ./custom
	mkdir lib
	cp $(BINDIR)/lib/$(LIBNAME).so ./lib/$(LIBNAME).so
	env GOOS=linux GOARCH=amd64  CGO_LDFLAGS="$(LINUX_RPATH) -L./lib -lcore" $(GOBUILDSRV) -o $(BINDIR)/$(CLINAME) ./cli/bydll
	rm -rf ./lib
	chmod +x $(BINDIR)/$(CLINAME)
	make webui


linux-custom:
	mkdir -p $(BINDIR)/
	#env GOARCH=mips $(GOBUILDSRV) -o $(BINDIR)/$(CLINAME) ./cli/
	go build -ldflags "-s -w" -trimpath -tags $(TAGS) -o $(BINDIR)/$(CLINAME) ./cli/
	chmod +x $(BINDIR)/$(CLINAME)
	make webui

macos-amd64:
	env GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-mmacosx-version-min=10.11" CGO_LDFLAGS="-mmacosx-version-min=10.11" CGO_ENABLED=1 go build -trimpath -tags $(TAGS),$(IOS_ADD_TAGS) -buildmode=c-shared -o $(BINDIR)/$(LIBNAME)-amd64.dylib ./custom
macos-arm64:
	env GOOS=darwin GOARCH=arm64 CGO_CFLAGS="-mmacosx-version-min=10.11" CGO_LDFLAGS="-mmacosx-version-min=10.11" CGO_ENABLED=1 go build -trimpath -tags $(TAGS),$(IOS_ADD_TAGS) -buildmode=c-shared -o $(BINDIR)/$(LIBNAME)-arm64.dylib ./custom
	
macos-universal: macos-amd64 macos-arm64 
	lipo -create $(BINDIR)/$(LIBNAME)-amd64.dylib $(BINDIR)/$(LIBNAME)-arm64.dylib -output $(BINDIR)/$(LIBNAME).dylib
	cp $(BINDIR)/$(LIBNAME).dylib ./$(LIBNAME).dylib 
	env GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-mmacosx-version-min=10.11" CGO_LDFLAGS="-mmacosx-version-min=10.11" CGO_LDFLAGS="bin/$(LIBNAME).dylib"  CGO_ENABLED=1 $(GOBUILDSRV)  -o $(BINDIR)/$(CLINAME) ./cli/bydll
	rm ./$(LIBNAME).dylib
	chmod +x $(BINDIR)/$(CLINAME)

clean:
	rm $(BINDIR)/* cli/bydll/cli.syso



# ---- VPN CLI + platform helpers (для TUN/“VPN-сервис”) ----------------------

.PHONY: vpncli
vpncli: ## собрать кроссплатформенный CLI (rvpncli)
	mkdir -p $(BINDIR)
	GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) go build -ldflags "-s -w" -trimpath -tags $(TAGS) \
		-o $(BINDIR)/$(VPNCLI_NAME) ./cmd/rvpncli
	@echo "built $(BINDIR)/$(VPNCLI_NAME)"

.PHONY: win-helper
win-helper: ## собрать Windows helper (elevated) для включения TUN (UAC)
	mkdir -p $(BINDIR)/windows
	GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
		go build -ldflags "-s -w -H=windowsgui" -trimpath -tags $(TAGS) \
		-o $(BINDIR)/windows/$(WIN_HELPER_NAME).exe ./windows/helper
	@echo "built $(BINDIR)/windows/$(WIN_HELPER_NAME).exe"
	@echo "NOTE: приложите манифест windows/helper/$(WIN_HELPER_NAME).exe.manifest в инсталлятор"

.PHONY: mac-helper
mac-helper: ## собрать macOS helper (LaunchDaemon) для start/stop TUN
	mkdir -p $(BINDIR)/macos
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -trimpath -tags $(TAGS) \
		-o $(BINDIR)/macos/$(MAC_HELPER_NAME) ./packaging/macos/helper
	@echo "built $(BINDIR)/macos/$(MAC_HELPER_NAME)"

.PHONY: desktop
desktop: vpncli win-helper mac-helper ## собрать rvpncli + helpers
	@echo "desktop helpers built: $(BINDIR)/$(VPNCLI_NAME), $(BINDIR)/windows/$(WIN_HELPER_NAME).exe, $(BINDIR)/macos/$(MAC_HELPER_NAME)"

release: # Create a new tag for release.	
	@bash -c '.github/change_version.sh'



