package main

import (
	"testing"
)

func testDiscoverPeers(t *testing.T){
  tr, err := NewTorrent("sample.torrent") 
  if err != nil{
    t.Error(err)
  }
  output, err := tr.discoverPeers()
  if err != nil{
    t.Error(err)
  }
  output[0].sendMessage(9)
  t.Log(output)
}
