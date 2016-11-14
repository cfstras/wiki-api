.PHONY: build all stop docker_image run deps generate
all: test build stop

docker_image: .make_docker_image

run: .make_run

deps: .make_deps

generate: .make_generate

.make_docker_image: docker/Dockerfile
	@rm .make_docker_image || :
	sudo docker build -t libgit2 docker/
	@touch .make_docker_image

.make_run: .make_docker_image
	[ -f .make_run ] && $(MAKE) stop || :
	$(shell echo "libgit-docker-"$$RANDOM > .make_run)
	sudo docker run -d --name $$(cat .make_run) \
	-v $$(pwd):/root/gopath/src/github.com/cfstras/wiki-api/ \
	libgit2 \
	bash -c 'while true; do sleep 10; done'

CMD=sudo docker exec -it $$(cat .make_run) bash -l -e -c

.make_deps: .make_run
	@rm .make_deps || :
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	go get -v github.com/jteeuwen/go-bindata/...'
	@touch .make_deps

.make_generate: .make_deps $(shell find . -type f -name "*.go") $(shell find data/ -type f)
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	go generate -v $$(go list ./... | grep -v /vendor/)'

test: .make_generate
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	go test -v $$(go list ./... | grep -v /vendor/) '
	sudo chown -R $$USER .

build: build/wiki-api.x64 build/wiki-crawl.x64

.PHONY: build/wiki-api.x64
build/wiki-api.x64: .make_generate
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	mkdir -p build && \
	go build -v -o build/wiki-api.x64 \
		--ldflags "-extldflags \"-static $$CGO_LDFLAGS\""'
	sudo chown -R $$USER .

.PHONY: build/wiki-crawl.x64
build/wiki-crawl.x64: .make_generate
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api/ && \
	mkdir -p build && \
	go build -v -o build/wiki-crawl.x64 \
		--ldflags "-extldflags \"-static $$CGO_LDFLAGS\"" ./wiki-crawl'
	sudo chown -R $$USER .

stop: .make_run
	sudo docker rm -f $$(cat .make_run)
	@rm .make_run
