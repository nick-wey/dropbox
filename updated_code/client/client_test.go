package main

import (
	"testing"

	"../lib/support/client"
	"../lib/support/rpc"
)

//assumes localhost:8080 as source

func TestFromClient(t *testing.T) {

	server = rpc.NewServerRemote("localhost:8080")
	c := Client{server}

	client.TestClient(t, &c)
    
}