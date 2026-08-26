package backend

// Policy is the exported refresh policy consumed by the frontend fixture.
type Policy struct {
	Force bool
}

// Refresh returns the session associated with a user.
func Refresh(userID string) string {
	return "session:" + userID
}
