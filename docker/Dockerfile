FROM ubuntu:12.04

RUN apt-get update -qq -y && apt-get install -y \
	wget ca-certificates cmake build-essential tar gzip pkg-config \
	ca-certificates git \
	libgpg-error-dev libssh2-1-dbg libssh2-1-dev libgcrypt-dev \
	&& \
	apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

RUN wget -q -O /bin/gimme https://raw.githubusercontent.com/travis-ci/gimme/master/gimme && \
	chmod a+x /bin/gimme && \
	eval "$(gimme 1.7)" && \
	mkdir /root/gopath

ENV GOPATH=/root/gopath
ENV PATH=$PATH:${GOPATH}/bin
RUN wget -q https://github.com/libgit2/libgit2/archive/v0.24.1.tar.gz && \
	tar xzf v*.tar.gz && \
	cd libgit* && mkdir build && cd build && \
	cmake -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=$HOME/libgit2/ \
		-DTHREADSAFE=ON -DBUILD_SHARED_LIBS=OFF -DBUILD_CLAR=OFF -DCMAKE_C_FLAGS=-fPIC \
		../ && \
	cmake --build . && \
	make install

RUN wget -q http://curl.haxx.se/download/curl-7.24.0.tar.lzma && \
	tar xf curl-7.24.0.tar.lzma && \
	cd curl-7.24.0 && \
	./configure --disable-shared --enable-static --prefix=$HOME/curl/ --disable-ldap --disable-sspi && \
	make -j2 && make install

#ENV PKG_CONFIG_PATH=/root/libgit2/lib/pkgconfig:/root/curl/lib/pkgconfig
ENV PKG_CONFIG_PATH=/root/libgit2/lib/pkgconfig
