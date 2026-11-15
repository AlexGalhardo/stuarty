package main

// The threats of a later phase and the tests they wait for. The table is data on
// purpose: a test that lands is deleted from here, and the check then requires it.

// laterThreats are the threats whose tests are not due yet, with the reason.
var laterThreats = map[string]string{}

// laterTests are tests a row names that are not due yet, with the reason: the
// half of a threat that belongs to a later phase.
var laterTests = map[string]string{}

// phaseOne are the threats of the Prototype and ALPHA phases (ROADMAP P1-SEC-001,
// SDD-007 section 2): the ones `-phase P1` checks.
var phaseOne = map[string]bool{
	"T-01": true, "T-02": true, "T-03": true, "T-04": true, "T-05": true,
	"T-10": true, "T-11": true, "T-12": true, "T-13": true, "T-14": true, "T-15": true, "T-16": true,
	"T-19": true, "T-22": true, "T-23": true, "T-24": true, "T-25": true,
	"T-28": true, "T-29": true, "T-30": true, "T-31": true,
}
