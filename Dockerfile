FROM golang:latest
MAINTAINER Vladislav Spirenkov <moiplov@gmail.com>


RUN mkdir -p /app
WORKDIR /app
COPY . /app/
RUN go get -d -v
RUN go build -o letsconsul

CMD ["./letsconsul"]
