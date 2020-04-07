#!/bin/sh

IP=$(hostname -i)
apt-get update
sleep 6000
/app/letsconsul -b $BIND_ADDRESS:$BIND_PORT -c $CONSUL_URL
