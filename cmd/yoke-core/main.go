// Command yoke-core is the Core: one process per instance, which supervises, routes and persists and
// never interprets a payload.
//
// In the service form it is executed with the composition in force and reads everything else from
// `core.yaml`. It runs the startup chain to readiness, serves until it is told to stop, and then undoes
// what it set up.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/core/trunk"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("yoke-core", flag.ContinueOnError)
	composition := flags.String("composition", "", "the composition document in force, overriding YOKE_COMPOSITION")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	// The composition in force is chosen now, and read when the deployment is.
	chosen := config.Composition(*composition, os.Getenv)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	st := &trunk.State{Form: trunk.Service, Env: os.Getenv, Stderr: os.Stderr, Composition: chosen}
	if err := trunk.Run(st, trunk.Steps()); err != nil {
		fmt.Fprintln(os.Stderr, "yoke-core:", err)
		return 1
	}
	<-stop
	if err := st.Stop(); err != nil {
		fmt.Fprintln(os.Stderr, "yoke-core:", err)
		return 1
	}
	return 0
}
