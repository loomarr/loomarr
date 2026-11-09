package storedb

// PendingTitle is the state of a titles row that exists only because a channel's lineup names the
// title (#2793): added by hand, nothing has asked for it yet. It lets channel_lineup.title_key be a
// foreign key. The store reads such a row as no record at all (GetTitle answers ErrNotFound, and
// counts and lists skip it), so the provisioning domain never sees the state; any real write of
// the title replaces it.
const PendingTitle = "pending"
