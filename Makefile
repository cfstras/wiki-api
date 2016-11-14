.PHONY: build all stop docker_image run deps download
all: test build stop

docker_image: .make_docker_image

run: .make_run

deps: .make_deps

download: .make_download

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

.make_download: .make_deps
	@rm .make_download || :
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	go generate -v $$(go list ./... | grep -v /vendor/) && \
	go get -d -t -v $$(go list ./... | grep -v /vendor/)'
	@touch .make_download

test: .make_download
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	go test -v $$(go list ./... | grep -v /vendor/) '
	sudo chown -R $$USER .

build: .make_download
	${CMD} 'cd /root/gopath/src/github.com/cfstras/wiki-api && \
	mkdir -p build && \
	go build -v -o build/wiki-api.x64 \
		--ldflags "-extldflags \"-static $$CGO_LDFLAGS\""'
	sudo chown -R $$USER .

stop: .make_run
	sudo docker rm -f $$(cat .make_run)
	@rm .make_run
