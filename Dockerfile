FROM alpine:edge AS builder
RUN apk add --no-cache go git
COPY mmdb.go /tmp/mmdb/
WORKDIR /tmp/mmdb
RUN go mod init mmdb && go get -u && go build && mkdir -p /mmdb && mv mmdb /mmdb/
RUN git clone https://github.com/maxmind/mmdbverify.git --depth 1 /tmp/mmdbverify/
WORKDIR /tmp/mmdbverify
RUN rm *.mod *.sum && go mod init mmdbverify && go get -u && go build && mv mmdbverify /mmdb/

FROM alpine:edge
RUN apk add --no-cache curl unzip git xz bind-tools
COPY build.sh /usr/bin/
COPY test.yaml /usr/bin/
COPY --from=builder /mmdb/* /usr/bin/
COPY --from=sliamb/prebuild-paopaodns /src/mosdns /usr/bin/
RUN chmod +x /usr/bin/*
WORKDIR /data
ENV MMDB_KEY=KEY
ENTRYPOINT [ "build.sh" ]
