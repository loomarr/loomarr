package httpcore

import "context"

// AuditActor is the admin's user id for the §3 audit trail (updated_by). The API_TOKEN
// break-glass path has no user, so it is empty (stored NULL), the same as a system write.
func AuditActor(ctx context.Context) string {
	if u, ok := UserFrom(ctx); ok {
		return u.ID
	}
	return ""
}
