// Package timeline is the cycle arithmetic over a channel's accepted schedule (design §9.1). It
// answers what airs at an instant (AiringAt), lists the broadcasts a guide shows between two times
// (BroadcastsBetween, BroadcastsWithPending), holds the window and carry-over rules that reconcile
// and the encoder share (WindowTurn, CarryOverEnd, KeepOnAir, OnAirChanged), and renders those
// broadcasts as XMLTV (RenderXMLTV).
//
// Every function is pure over a []schedule.Slot and its epoch, so the encoder, the guide and
// reconcile get one answer from one walk. It knows nothing about stores, settings, encoders or
// HTTP; internal/playout's packager asks it what is on.
package timeline
