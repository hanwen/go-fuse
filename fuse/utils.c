int receive_fuse_conn(int fd, int* fuse_fd) {
  if (fd < 0) {
  	return -1;
  }
  *fuse_fd = -1;
  return -1;
}
