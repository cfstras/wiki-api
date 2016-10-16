.PHONY: build all stop
all: test build stop

_make_docker_image: docker/Dockerfile
	@rm _make_docker_image || :
	sudo docker build -t libgit2 docker/
	@touch _make_docker_image

_make_run: _make_docker_image
	[ -f _make_run ] && $(MAKE) stop || :
	$(shell echo "libgit-docker-"$$RANDOM > _make_run)
	sudo docker run -d --name $$(cat _make_run) \
	-v $$(pwd):/root/gopath/src/github.com/cfstras/wiki-api/ \
	libgit2 \
	bash -c 'while true; do sleep 10; done'

CMD=sudo docker exec -it $$(cat _make_run) bash -l -e -c

_make_deps: _make_run
	@rm _make_deps || :
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	go get -v github.com/jteeuwen/go-bindata/...'
	@touch _make_deps

_make_download: _make_deps
	@rm _make_download || :
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	go generate -v ./... && \
	go get -d -t -v ./...'
	@touch _make_download

test: _make_download
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	go test -v ./...'
	sudo chown -R $$USER .

build: _make_download
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	mkdir -p build && \
	go build -v -o build/wiki-api.x64 \
		--ldflags "-extldflags \"-static $$CGO_LDFLAGS\""'
	sudo chown -R $$USER .

stop: _make_run
	sudo docker rm -f $$(cat _make_run)
	@rm _make_run
