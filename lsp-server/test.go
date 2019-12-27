package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/go-language-server/jsonrpc2"
	"go.uber.org/zap"

	"github.com/zoncoen/scenarigo/lsp"
)

func main() {
	rawJSON := []byte(`{
	  "level": "debug",
	  "encoding": "json",
	  "outputPaths": ["/Users/zoncoen/logs/scenarigo.log"],
	  "errorOutputPaths": ["/Users/zoncoen/logs/scenarigo.log"],
	  "encoderConfig": {
	    "messageKey": "message",
	    "levelKey": "level",
	    "levelEncoder": "lowercase"
	  }
	}`)

	var cfg zap.Config
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		panic(err)
	}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	stream := jsonrpc2.NewStream(os.Stdin, os.Stdout)
	srv, err := lsp.NewServer(context.Background(), stream, lsp.WithLogger(logger))
	if err != nil {
		panic(err)
	}
	srv.Run(context.Background())
}
