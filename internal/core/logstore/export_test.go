package logstore

// Break makes the store's connection to its file fail, as a disk that went away would.
func Break(s *Store) { s.brk() }

// Vacuums is how many times the store has been vacuumed.
func Vacuums(s *Store) int { return s.vacuumed() }
