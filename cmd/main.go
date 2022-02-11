package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/log"
	serverpkg "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/envoyproxy/go-control-plane/pkg/test/v3"
	"github.com/maycommit/envoy-control-plane/internal/server"
)

var (
	port   uint
	nodeID string
)

func main() {
	logger := log.NewDefaultLogger()
	newCache := cache.NewSnapshotCache(false, cache.IDHash{}, logger)

	cb := &test.Callbacks{Debug: true}

	srv := serverpkg.NewServer(context.Background(), newCache, cb)

	server.Run(context.Background(), srv, 18000)
}

func (m *Main) loop() {
	t := time.NewTicker(3 * time.Second)

	for {
		select {
		case <-t.C:
			data, err := ioutil.ReadFile("/etc/script.json")
			if err != nil {
				panic(err)
			}

			inputList := []Input{}
			err = json.Unmarshal(data, &inputList)
			if err != nil {
				panic(err)
			}

			fmt.Println(inputList)
			for _, i := range inputList {
				nodeSnapshot := NewNodeSnapshot(
					i.Version,
					"test-cluster",
					i.Node,
					i.Host,
					i.Port,
					i.Domains,
				)
				snapshot := nodeSnapshot.GenerateSnapshot()
				err := snapshot.Consistent()
				if err != nil {
					panic(err)
				}

				err = m.currentCache.SetSnapshot(context.Background(), i.Node, snapshot)
				if err != nil {
					panic(err)
				}
			}
		}
	}
}
