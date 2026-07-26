// Package core is the pure state machine of agentsdk.
//
// Core has zero vendor dependencies and uses only the Go standard library.
// It defines state transitions, events, instructions, and the ports executed
// by the runtime shell; it does not perform I/O itself.
//
// Per the project convention, package-level constants use
// SCREAMING_SNAKE_CASE; staticcheck ST1003 needs an exclusion for constants.
package core
