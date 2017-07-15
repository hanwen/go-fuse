package fuse

// Go 1.9 introduces polling for file I/O. The implementation causes
// the runtime's epoll to take up the last GOMAXPROCS slot, and if
// that happens, we won't have any threads left to service FUSE's
// _OP_POLL request. Prevent this by forcing _OP_POLL to happen, so we
// can say ENOSYS and prevent further _OP_POLL requests.
const pollHackName = ".go-fuse-epoll-hack"
const pollHackInode = ^uint64(0)
