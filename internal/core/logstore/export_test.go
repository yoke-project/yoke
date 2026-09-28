package logstore

// Break makes the store's connection to its file fail, as a disk that went away would.
func Break(s *Store) { s.brk() }
