package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cko-recruitment/payment-gateway-challenge-go/docs"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/api"
)

type buildMetadata struct {
	version string
	commit  string
	date    string
}

func resolveBuildMetadata(getenv func(string) string) buildMetadata {
	metadata := buildMetadata{
		version: getenv("APP_VERSION"),
		commit:  getenv("APP_COMMIT"),
		date:    getenv("APP_DATE"),
	}
	if metadata.version == "" {
		metadata.version = "dev"
	}
	if metadata.commit == "" {
		metadata.commit = "none"
	}
	if metadata.date == "" {
		metadata.date = "unknown"
	}
	return metadata
}

//	@title			Payment Gateway Challenge Go
//	@description	Interview challenge for building a Payment Gateway - Go version

//	@host		localhost:8090
//	@BasePath	/

// @securityDefinitions.basic	BasicAuth
func main() {
	metadata := resolveBuildMetadata(os.Getenv)
	fmt.Printf("version %s, commit %s, built at %s\n", metadata.version, metadata.commit, metadata.date)
	docs.SwaggerInfo.Version = metadata.version

	err := run()
	if err != nil {
		fmt.Printf("fatal API error: %v\n", err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// graceful shutdown
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		fmt.Printf("sigterm/interrupt signal\n")
		cancel()
	}()

	defer func() {
		// recover after panic
		if x := recover(); x != nil {
			fmt.Printf("run time panic:\n%v\n", x)
			panic(x)
		}
	}()

	server, err := api.New()
	if err != nil {
		return err
	}
	if err := server.Run(ctx, ":8090"); err != nil {
		return err
	}

	return nil
}
