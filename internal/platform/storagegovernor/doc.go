// Package storagegovernor owns host-capacity policy and atomic reservations for
// Loomarr-managed writes. Callers provide a destination and conservative byte
// estimate; the governor keeps filesystem identity, managed usage, hard host
// reserve, soft library allowance, and concurrent work behind one interface.
package storagegovernor
