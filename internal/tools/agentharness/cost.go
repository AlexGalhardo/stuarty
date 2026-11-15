package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// perTurnGrowth is the tokens each turn adds to the context at worst: the tool call, and the
// output the CLI returns (capped at maxOutputBytes, about 2000 tokens).
const perTurnGrowth = 2600

type costEstimate struct {
	input, output int
	usd           float64
}

// estimate is the worst case: every run uses all its turns, every turn re-sends the whole context
// and writes max-tokens. The real cost is usually a small fraction of it.
func estimate(o options, systemBytes int) costEstimate {
	system := systemBytes/4 + 200

	var perRun costEstimate

	for turn := 1; turn <= o.maxTurns; turn++ {
		perRun.input += system + 300 + turn*perTurnGrowth
		perRun.output += o.maxTokens
	}

	n := o.runs * len(o.scenarioList)
	e := costEstimate{input: perRun.input * n, output: perRun.output * n}
	e.usd = float64(e.input)/1e6*o.priceIn + float64(e.output)/1e6*o.priceOut

	return e
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

// asExit reports the exit code when err is a command that ran and failed.
func asExit(err error) (int, bool) {
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode(), true
	}

	return 0, false
}
