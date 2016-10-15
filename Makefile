.PHONY: build all
all: docker-image build

docker-image: docker/Dockerfile
	sudo docker build -t libgit2 docker/

build:
	sudo docker run -it --rm -v /tmp/gopath.1:/root/gopath \
	-v $$(pwd):/root/gopath/src/github.com/cfstras/wiki-api/ \
	libgit2 \
	bash -c 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	eval "$$(gimme 1.7)" && \
	go get -v github.com/jteeuwen/go-bindata/... && \
	go generate -v ./... && \
	go get -d -t -v ./... && \
	mkdir -p build && \
	export CGO_CFLAGS=$$(pkg-config --cflags --static libgit2 libssh2) && \
	export CGO_LDFLAGS="$$(pkg-config --libs --static libgit2 libssh2) \
		-lgcrypt -lgpg-error" && \
	go test -v ./... && \
	go get -v \
		--ldflags "-extldflags \"-static $$CGO_LDFLAGS\"" && \
	go build -v -o build/wiki-api.x64 \
		--ldflags "-extldflags \"-static $$CGO_LDFLAGS\""'
	sudo chown -R $$USER .
