package main

// Generate the per-component config resources from the shelly-go IR
// (which is itself generated from the Shelly API docs). Run before tfplugindocs
// so the generated resources are included in the documentation.
//
//go:generate go run ./internal/provider/gen
