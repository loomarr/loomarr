// Package proctree supervises one child process and every descendant it starts.
//
// The supervisor owns the complete lifecycle: it configures the platform tree before
// starting the command, attaches the started process without a child-creation race, binds
// cancellation to tree teardown, and makes Stop and Wait safe to call concurrently. Unix
// uses a process group; Windows uses a Job Object.
package proctree
