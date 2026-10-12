// Command yoke-core is the Core: one process per instance, which supervises, routes and persists and
// never interprets a payload.
//
// In the service form it is executed with the composition in force and reads everything else from
// `core.yaml`. In the application form the invocation is the whole of what it receives: the instance's
// name, the bundle, the engine, the account's retention ceiling and the resolved parameter values. It runs
// the startup chain to readiness, serves until it is told to stop, and then undoes what it set up.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/trunk"
	"github.com/yoke-project/yoke/internal/gate"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("yoke-core", flag.ContinueOnError)
	composition := flags.String("composition", "", "the composition document in force, overriding YOKE_COMPOSITION")
	admitUnlaunched := flags.Bool("admit-unlaunched", false, "a development run: a process this Core did not launch may register")
	name := flags.String("instance", "", "the application form: the instance's name, which every path is derived from")
	bundle := flags.String("bundle", "", "the application form: where the descriptor and the components are")
	engineAt := flags.String("engine", "", "the application form: the container engine's address")
	unverified := flags.Bool("unverified", false, "the application form: a run from a working directory, whose bundle was not verified")
	var retention, params repeated
	flags.Var(&retention, "retention", "the application form: the account's ceiling, `<limit>=<value>`, once per limit")
	flags.Var(&params, "param", "the application form: a resolved parameter value, `<group>.<name>=<value>`, once per value")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	st := &trunk.State{Form: trunk.Service, Env: os.Getenv, Stderr: os.Stderr, AdmitUnlaunched: *admitUnlaunched}
	if *name == "" {
		if *bundle != "" || *engineAt != "" || *unverified || len(retention) > 0 || len(params) > 0 {
			return refuse(errors.New("--bundle, --engine, --retention, --param and --unverified belong to the application form, which --instance names"))
		}
		// The composition in force is chosen now, and read when the deployment is.
		st.Composition = config.Composition(*composition, os.Getenv)
	} else {
		if *composition != "" {
			return refuse(errors.New("--composition belongs to the service form, and --instance names the application form"))
		}
		if *bundle == "" {
			return refuse(errors.New("the application form needs --bundle, where the descriptor is"))
		}
		ceiling, err := ceilingOf(retention)
		if err != nil {
			return refuse(err)
		}
		values, err := valuesOf(params)
		if err != nil {
			return refuse(err)
		}
		st.Form, st.Name, st.Bundle, st.Engine, st.Ceiling, st.Values, st.Unverified =
			trunk.Application, *name, *bundle, *engineAt, ceiling, values, *unverified
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

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

// refuse says why an invocation does not hold, and is the exit of one that does not parse.
func refuse(err error) int {
	fmt.Fprintln(os.Stderr, "yoke-core:", err)
	return 2
}

// repeated is a flag given once per value.
type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

// ceilingOf reads `--retention <limit>=<value>`: `age` a duration, `bytes` a size, `entries` a count, each
// at most once. Zero constrains nothing.
func ceilingOf(written []string) (logstore.Limits, error) {
	var l logstore.Limits
	seen := map[string]bool{}
	for _, w := range written {
		limit, value, ok := strings.Cut(w, "=")
		if !ok {
			return l, fmt.Errorf("--retention %q is not <limit>=<value>", w)
		}
		if seen[limit] {
			return l, fmt.Errorf("--retention names %s twice", limit)
		}
		seen[limit] = true
		switch limit {
		case "age":
			d, ok := gate.ParseDuration(value)
			if !ok && value != "0" {
				return l, fmt.Errorf("--retention age=%s is not an integer followed by s, m, h or d", value)
			}
			l.Age = d
		case "bytes":
			n, ok := gate.ParseSize(value)
			if !ok {
				return l, fmt.Errorf("--retention bytes=%s is not an integer followed by KB, MB or GB", value)
			}
			l.Bytes = uint64(n)
		case "entries":
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return l, fmt.Errorf("--retention entries=%s is not a count", value)
			}
			l.Entries = n
		default:
			return l, fmt.Errorf("--retention %s is none of age, bytes and entries", limit)
		}
	}
	return l, nil
}

// valuesOf reads `--param <group>.<name>=<value>`, each at most once.
func valuesOf(written []string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, w := range written {
		key, value, ok := strings.Cut(w, "=")
		group, name, dotted := strings.Cut(key, ".")
		if !ok || !dotted || group == "" || name == "" {
			return nil, fmt.Errorf("--param %q is not <group>.<name>=<value>", w)
		}
		if _, twice := out[group][name]; twice {
			return nil, fmt.Errorf("--param names %s twice", key)
		}
		if out[group] == nil {
			out[group] = map[string]string{}
		}
		out[group][name] = value
	}
	return out, nil
}
