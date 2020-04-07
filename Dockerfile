FROM golang:latest
MAINTAINER Vladislav Spirenkov <moiplov@gmail.com>

ARG renew_interval=168h
ARG reload_interval=10s
ARG domains_enabled=wasp2-dev.myapp.de
ARG email=christoph.heuwieser@germanedge.com
ARG consul_url=consul:8500
ARG bind_port=8080
ARG bind_address=0.0.0.0

ENV RENEW_INTERVAL=$renew_interval \
    RELOAD_INTERVAL=$reload_interval \
    DOMAINS_ENABLED=$domains_enabled \
    EMAIL=$email \
    CONSUL_URL=$consul_url \
    BIND_PORT=$bind_port \
    BIND_ADDRESS=$bind_address
    

RUN mkdir -p /app
WORKDIR /app
COPY . /app/
RUN go get -d -v
RUN go build -o letsconsul

ADD entrypoint.sh /
RUN chmod +x /entrypoint.sh

CMD ["/entrypoint.sh"]
